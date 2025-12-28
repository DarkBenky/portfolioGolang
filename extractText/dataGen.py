import json
import random
import numpy as np
from tokenizers import Tokenizer

class DataGenerator:
    def __init__(self, path_to_dataset, path_to_tokenizer):
        self.tokenizer = Tokenizer.from_file(path_to_tokenizer)
        self.vocab_size = self.tokenizer.get_vocab_size()
        
        with open(path_to_dataset, 'r', encoding='utf-8') as f:
            texts = json.load(f)
        
        self.valid_texts = [text for text in texts if len(text['tokenizedText']) > 1]
        
        if not self.valid_texts:
            raise ValueError("No valid texts found with length > 1")
        
        print(f"Loaded {len(self.valid_texts)} valid texts")
        print(f"Vocabulary size: {self.vocab_size}")
    
    def _prepare_sequence(self, tokenized, max_sample_length):
        # Need at least 2 tokens (1 input, 1 target)
        if len(tokenized) < 2:
            return None, None
        
        # For language modeling, we take a chunk and predict next tokens
        # If text is shorter than max_sample_length+1, use what we have
        # Otherwise, take a random chunk
        if len(tokenized) <= max_sample_length + 1:
            chunk = tokenized
        else:
            # Random starting position
            start_idx = random.randint(0, len(tokenized) - max_sample_length - 1)
            chunk = tokenized[start_idx:start_idx + max_sample_length + 1]
        
        # Input: all tokens except the last
        # Target: all tokens except the first (shifted by 1)
        input_seq = chunk[:-1]
        target_seq = chunk[1:]
        
        # Pad if needed
        if len(input_seq) < max_sample_length:
            pad_len = max_sample_length - len(input_seq)
            input_seq = [0] * pad_len + input_seq
            target_seq = [0] * pad_len + target_seq
        
        return input_seq, target_seq
    
    def generate_samples(self, num_of_samples, max_sample_length):
        samples_generated = 0
        while samples_generated < num_of_samples:
            text = random.choice(self.valid_texts)
            input_seq, target_seq = self._prepare_sequence(text['tokenizedText'], max_sample_length)
            
            if input_seq is None:
                continue
            
            samples_generated += 1
            yield (np.array(input_seq, dtype=np.int32), np.array(target_seq, dtype=np.int32))
    
    def generate_batches(self, batch_size, max_sample_length):
        while True:
            X_batch = []
            Y_batch = []
            
            while len(X_batch) < batch_size:
                text = random.choice(self.valid_texts)
                input_seq, target_seq = self._prepare_sequence(text['tokenizedText'], max_sample_length)
                
                if input_seq is None:
                    continue
                
                X_batch.append(input_seq)
                Y_batch.append(target_seq)
            
            yield (np.array(X_batch, dtype=np.int32), np.array(Y_batch, dtype=np.int32))

    def generate_one_hot_batches(self, batch_size, max_sample_length):
        """
        Legacy method - not recommended for language modeling.
        Use generate_batches() instead with SparseCategoricalCrossentropy.
        """
        while True:
            X_batch = []
            Y_batch = []

            while len(X_batch) < batch_size:
                text = random.choice(self.valid_texts)
                input_seq, target_seq = self._prepare_sequence(
                    text['tokenizedText'], max_sample_length
                )

                if input_seq is None:
                    continue

                X_batch.append(input_seq)

                # One-hot encode the entire target sequence
                y_one_hot = np.zeros((max_sample_length, self.vocab_size), dtype=np.float32)
                for i, token_id in enumerate(target_seq):
                    y_one_hot[i, token_id] = 1.0
                Y_batch.append(y_one_hot)

            yield (
                np.array(X_batch, dtype=np.int32),
                np.array(Y_batch, dtype=np.float32)
            )


# Example usage:
if __name__ == "__main__":
    data_gen = DataGenerator('extractedTexts.json', 'tokenizer.json')
    
    print("\n=== Testing generate_batches ===")
    for X, Y in data_gen.generate_batches(batch_size=2, max_sample_length=10):
        print("Input batch shape:", X.shape)
        print("Target batch shape:", Y.shape)
        print("\nExample from batch:")
        print("Input sequence:", X[0])
        print("Target sequence:", Y[0])
        print("\nNotice how targets are inputs shifted left by 1 position")
        break