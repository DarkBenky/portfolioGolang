import os
os.environ['TF_XLA_FLAGS'] = '--tf_xla_auto_jit=0'
os.environ['XLA_FLAGS'] = '--xla_gpu_cuda_data_dir=/usr/local/cuda'

import tensorflow as tf
from tensorflow.keras import mixed_precision
import numpy as np
from tokenizers import Tokenizer
from train import build_language_model

gpus = tf.config.list_physical_devices('GPU')
if gpus:
    try:
        for gpu in gpus:
            tf.config.experimental.set_memory_growth(gpu, True)
    except RuntimeError as e:
        print(f"GPU configuration: {e}")

policy = mixed_precision.Policy('mixed_float16')
mixed_precision.set_global_policy(policy)


class TextGenerator:
    def __init__(self, weights_path, tokenizer_path, context_window=2048, d_model=1152, num_heads=18, num_layers=44, ffn_dim=None, dropout_rate=0.1):
        self.tokenizer = Tokenizer.from_file(tokenizer_path)
        self.vocab_size = self.tokenizer.get_vocab_size()
        self.context_window = context_window
        
        if ffn_dim is None:
            ffn_dim = d_model * 4
        
        print(f"Building model architecture...")
        self.model = build_language_model(
            vocab_size=self.vocab_size,
            context_window=context_window,
            d_model=d_model,
            num_heads=num_heads,
            num_layers=num_layers,
            ffn_dim=ffn_dim,
            dropout_rate=dropout_rate
        )
        
        self.model.build(input_shape=(None, context_window))
        
        print(f"Loading weights from {weights_path}...")
        self.model.load_weights(weights_path)
        print(f"Model loaded successfully! Total parameters: {self.model.count_params():,}")
        
        print("Compiling optimized inference function...")
        self._predict_fn = tf.function(
            self._predict_step,
            input_signature=[tf.TensorSpec(shape=[1, None], dtype=tf.int32)]
        )
        print("Warming up model...")
        dummy_input = tf.constant([[1, 2, 3]], dtype=tf.int32)
        _ = self._predict_fn(dummy_input)
        print("Ready for fast inference!\n")
    
    @tf.function
    def _predict_step(self, input_seq):
        predictions = self.model(input_seq, training=False)
        return predictions
    
    def generate(self, prompt, max_length=100, temperature=0.8, top_k=40):
        tokens = self.tokenizer.encode(prompt).ids
        
        print(f"Starting generation (max {max_length} tokens)...")
        
        for i in range(max_length):
            input_seq = tokens[-self.context_window:]
            input_tensor = tf.constant([input_seq], dtype=tf.int32)
            
            try:
                predictions = self._predict_fn(input_tensor)
                next_token_logits = predictions[0, -1, :].numpy()
            except Exception as e:
                print(f"\nPrediction error: {e}")
                break
            
            if temperature > 0:
                next_token_logits = next_token_logits / temperature
                
                top_k_indices = np.argpartition(next_token_logits, -top_k)[-top_k:]
                top_k_logits = next_token_logits[top_k_indices]
                
                top_k_logits = top_k_logits - np.max(top_k_logits)
                probs = np.exp(top_k_logits)
                probs = probs / np.sum(probs)
                
                next_token = np.random.choice(top_k_indices, p=probs)
            else:
                next_token = np.argmax(next_token_logits)
            
            tokens.append(int(next_token))
            
            decoded = self.tokenizer.decode([next_token])
            if '<EOS>' in decoded or decoded.strip() in ['</s>', '<|endoftext|>']:
                break
            
            if i % 50 == 0 and i > 0:
                print(f"Generated {i} tokens...")
                
        return self.tokenizer.decode(tokens)


def main():
    print("Loading model...")
    generator = TextGenerator(
        weights_path='best_model.weights.h5',
        tokenizer_path='tokenizer.json',
        context_window=2048,
        d_model=1152,
        num_heads=18,
        num_layers=44,
        dropout_rate=0.1
    )
    print("Model loaded successfully\n")
    
    print("Commands:")
    print("  Type your prompt and press Enter")
    print("  Type 'quit' or 'exit' to stop")
    print("  Type 'clear' to clear screen\n")
    
    while True:
        try:
            prompt = input("\nYou: ").strip()
            
            if prompt.lower() in ['quit', 'exit']:
                print("Goodbye!")
                break
                
            if prompt.lower() == 'clear':
                print("\n" * 50)
                continue
                
            if not prompt:
                continue
            
            print("\nGenerating...")
            import time
            start_time = time.time()
            response = generator.generate(prompt, max_length=256, temperature=0.8, top_k=40)
            elapsed = time.time() - start_time
            
            num_tokens = len(generator.tokenizer.encode(response).ids)
            tokens_per_sec = num_tokens / elapsed if elapsed > 0 else 0
            
            print("\nAssistant:", response)
            print(f"\n[Generated {num_tokens} tokens in {elapsed:.2f}s ({tokens_per_sec:.1f} tokens/sec)]")
            
        except KeyboardInterrupt:
            print("\n\nGoodbye!")
            break
        except Exception as e:
            print(f"\nError: {e}")


if __name__ == '__main__':
    main()