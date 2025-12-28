import tensorflow as tf
import numpy as np
from tokenizers import Tokenizer


class TextGenerator:
    def __init__(self, model_path, tokenizer_path):
        self.model = tf.keras.models.load_model(model_path)
        self.tokenizer = Tokenizer.from_file(tokenizer_path)
        self.vocab_size = self.tokenizer.get_vocab_size()
        
    def generate(self, prompt, max_length=100, temperature=0.8, top_k=40):
        tokens = self.tokenizer.encode(prompt).ids
        
        for _ in range(max_length):
            input_seq = np.array([tokens], dtype=np.int32)
            
            predictions = self.model.predict(input_seq, verbose=0)
            next_token_logits = predictions[0, -1, :]
            
            next_token_logits = next_token_logits / temperature
            
            top_k_indices = np.argsort(next_token_logits)[-top_k:]
            top_k_logits = next_token_logits[top_k_indices]
            
            probs = np.exp(top_k_logits) / np.sum(np.exp(top_k_logits))
            next_token = np.random.choice(top_k_indices, p=probs)
            
            tokens.append(int(next_token))
            
            decoded = self.tokenizer.decode([next_token])
            if '<EOS>' in decoded:
                break
                
        return self.tokenizer.decode(tokens)


def main():
    print("Loading model...")
    generator = TextGenerator('best_language_model.keras', 'tokenizer.json')
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
            response = generator.generate(prompt, max_length=150, temperature=0.8, top_k=40)
            print("\nAssistant:", response)
            
        except KeyboardInterrupt:
            print("\n\nGoodbye!")
            break
        except Exception as e:
            print(f"\nError: {e}")


if __name__ == '__main__':
    main()