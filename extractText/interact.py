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
        
    def generate(self, prompt, max_length=100, temperature=0.8, top_k=40):
        tokens = self.tokenizer.encode(prompt).ids
        
        print(f"Starting generation (max {max_length} tokens)...")
        
        for i in range(max_length):
            input_seq = np.array([tokens[-2048:]], dtype=np.int32)
            
            try:
                predictions = self.model.predict(input_seq, verbose=0, batch_size=1)
            except Exception as e:
                print(f"\nPrediction error: {e}")
                print("Attempting to continue...")
                break
            
            next_token_logits = predictions[0, -1, :]
            
            next_token_logits = next_token_logits / temperature
            
            top_k_indices = np.argsort(next_token_logits)[-top_k:]
            top_k_logits = next_token_logits[top_k_indices]
            
            probs = np.exp(top_k_logits) / np.sum(np.exp(top_k_logits))
            next_token = np.random.choice(top_k_indices, p=probs)
            
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
            response = generator.generate(prompt, max_length=32, temperature=0.8, top_k=10)
            print("\nAssistant:", response)
            
        except KeyboardInterrupt:
            print("\n\nGoodbye!")
            break
        except Exception as e:
            print(f"\nError: {e}")


if __name__ == '__main__':
    main()