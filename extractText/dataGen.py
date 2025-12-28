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
        max_position = len(tokenized) - 1
        if max_position < 1:
            return None, None
        
        position = random.randint(1, max_position)
        input_seq = tokenized[:position]
        next_token = tokenized[position]
        
        if len(input_seq) < max_sample_length:
            input_seq = [0] * (max_sample_length - len(input_seq)) + input_seq
        else:
            input_seq = input_seq[-max_sample_length:]
        
        return input_seq, next_token
    
    def generate_samples(self, num_of_samples, max_sample_length):
        samples_generated = 0
        while samples_generated < num_of_samples:
            text = random.choice(self.valid_texts)
            input_seq, next_token = self._prepare_sequence(text['tokenizedText'], max_sample_length)
            
            if input_seq is None:
                continue
            
            samples_generated += 1
            yield (np.array(input_seq, dtype=np.int32), np.array(next_token, dtype=np.int32))
    
    def generate_batches(self, batch_size, max_sample_length):
        while True:
            X_batch = []
            Y_batch = []
            
            while len(X_batch) < batch_size:
                text = random.choice(self.valid_texts)
                input_seq, next_token = self._prepare_sequence(text['tokenizedText'], max_sample_length)
                
                if input_seq is None:
                    continue
                
                X_batch.append(input_seq)
                Y_batch.append(next_token)
            
            yield (np.array(X_batch, dtype=np.int32), np.array(Y_batch, dtype=np.int32))

# Example usage:
# data_gen = DataGenerator('extractedTexts.json', 'tokenizer.json')
# for X, Y in data_gen.generate_samples(5, 10):
#     print("Input sequence:", X)
#     print("Next token:", Y)