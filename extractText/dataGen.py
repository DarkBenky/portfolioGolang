import json
import random
import numpy as np
from tokenizers import Tokenizer

class DataGenerator:
    def __init__(self, dataset_paths, path_to_tokenizer, cache_size=1):
        """
        Args:
            dataset_paths: List of paths to dataset JSON files
            path_to_tokenizer: Path to tokenizer file
            cache_size: Number of files to keep in memory (default: 1)
        """
        self.tokenizer = Tokenizer.from_file(path_to_tokenizer)
        self.vocab_size = self.tokenizer.get_vocab_size()
        self.dataset_paths = dataset_paths if isinstance(dataset_paths, list) else [dataset_paths]
        self.cache_size = cache_size
        
        # Cache for loaded files: {file_path: valid_texts_list}
        self.file_cache = {}
        self.cache_order = []  # Track order for LRU eviction
        
        # Count valid texts in each file
        self.file_text_counts = []
        total_valid = 0
        
        for path in self.dataset_paths:
            count = self._count_valid_texts(path)
            self.file_text_counts.append(count)
            total_valid += count
            print(f"File: {path} - {count} valid texts")
        
        if total_valid == 0:
            raise ValueError("No valid texts found in any dataset file")
        
        # Create weights for file selection based on number of valid texts
        self.file_weights = [count / total_valid for count in self.file_text_counts]
        
        print(f"\nTotal valid texts across all files: {total_valid}")
        print(f"Vocabulary size: {self.vocab_size}")
        print(f"Cache size: {cache_size} file(s)")
    
    def _count_valid_texts(self, file_path):
        """Count valid texts (length > 1) without loading entire file into memory"""
        count = 0
        with open(file_path, 'r', encoding='utf-8') as f:
            texts = json.load(f)
            for text in texts:
                if len(text['tokenizedText']) > 1:
                    count += 1
        return count
    
    def _load_file_to_cache(self, file_path):
        """Load a file into cache with LRU eviction"""
        # If already in cache, move to end (most recently used)
        if file_path in self.file_cache:
            self.cache_order.remove(file_path)
            self.cache_order.append(file_path)
            return
        
        # Load file and extract only tokenized arrays (memory efficient!)
        with open(file_path, 'r', encoding='utf-8') as f:
            texts = json.load(f)
        
        # Extract only token arrays, not full text objects
        tokenized_arrays = [
            text['tokenizedText'] 
            for text in texts 
            if len(text['tokenizedText']) > 1
        ]
        
        # Add to cache
        self.file_cache[file_path] = tokenized_arrays
        self.cache_order.append(file_path)
        
        # Evict oldest if cache is full
        if len(self.file_cache) > self.cache_size:
            oldest = self.cache_order.pop(0)
            del self.file_cache[oldest]
            print(f"Evicted {oldest} from cache")
    
    def _get_random_text_from_cached_file(self, file_path):
        """Get a random tokenized array from a cached file"""
        if file_path not in self.file_cache:
            self._load_file_to_cache(file_path)
        
        tokenized_arrays = self.file_cache[file_path]
        if not tokenized_arrays:
            return None
        
        return random.choice(tokenized_arrays)
    
    def _get_random_text(self):
        """Get a random tokenized array from a randomly selected file (weighted by file size)"""
        # Select file based on weights
        file_path = random.choices(self.dataset_paths, weights=self.file_weights, k=1)[0]
        return self._get_random_text_from_cached_file(file_path)
    
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
            tokenized = self._get_random_text()
            
            if tokenized is None:
                continue
            
            input_seq, target_seq = self._prepare_sequence(tokenized, max_sample_length)
            
            if input_seq is None:
                continue
            
            samples_generated += 1
            yield (np.array(input_seq, dtype=np.int32), np.array(target_seq, dtype=np.int32))
    
    def generate_batches(self, batch_size, max_sample_length):
        while True:
            X_batch = []
            Y_batch = []
            
            while len(X_batch) < batch_size:
                tokenized = self._get_random_text()
                
                if tokenized is None:
                    continue
                
                input_seq, target_seq = self._prepare_sequence(tokenized, max_sample_length)
                
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
                tokenized = self._get_random_text()
                
                if tokenized is None:
                    continue
                
                input_seq, target_seq = self._prepare_sequence(
                    tokenized, max_sample_length
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
    
    def preload_all_files(self):
        """Preload all files into cache (use if you have enough RAM)"""
        print("\nPreloading all files into cache...")
        self.cache_size = len(self.dataset_paths)
        for path in self.dataset_paths:
            self._load_file_to_cache(path)
        print("All files loaded into cache!")


# Example usage:
if __name__ == "__main__":
    # Option 1: Cache one file at a time (memory efficient)
    data_gen = DataGenerator(
        ['extractedTexts_multilang.json', 'extractedTexts.json'],
        'tokenizer.json',
        cache_size=1
    )
    
    # Option 2: Cache multiple files (faster, more memory)
    # data_gen = DataGenerator(
    #     ['extractedTexts_multilang.json', 'extractedTexts.json'],
    #     'tokenizer.json',
    #     cache_size=2  # Keep both files in memory
    # )
    
    # Option 3: Preload all files (fastest, most memory)
    # data_gen = DataGenerator(
    #     ['extractedTexts_multilang.json', 'extractedTexts.json'],
    #     'tokenizer.json'
    # )
    # data_gen.preload_all_files()
    
    print("\n=== Testing generate_batches ===")
    for i, (X, Y) in enumerate(data_gen.generate_batches(batch_size=2, max_sample_length=10)):
        print(f"\nBatch {i+1}:")
        print("Input batch shape:", X.shape)
        print("Target batch shape:", Y.shape)
        print("Example input sequence:", X[0])
        print("Example target sequence:", Y[0])
        
        if i >= 2:  # Show 3 batches
            break