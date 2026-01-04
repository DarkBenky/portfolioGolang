import json
import random
import numpy as np
import os
import requests
from tokenizers import Tokenizer

try:
    import ijson
    HAS_IJSON = True
except ImportError:
    HAS_IJSON = False

class DataGenerator:
    def __init__(self, dataset_paths, path_to_tokenizer, cache_size=1, lazy_count=True, count_cache_file='.dataset_counts.json'):
        """
        Args:
            dataset_paths: List of paths to dataset JSON files
            path_to_tokenizer: Path to tokenizer file
            cache_size: Number of files to keep in memory (default: 1)
            lazy_count: If True, use equal weights initially (faster, less memory).
                       If False, count all texts upfront (slower, more accurate weights)
            count_cache_file: File to cache counts to avoid re-counting on subsequent runs
        """
        self.tokenizer = Tokenizer.from_file(path_to_tokenizer)
        self.vocab_size = self.tokenizer.get_vocab_size()
        self.dataset_paths = dataset_paths if isinstance(dataset_paths, list) else [dataset_paths]
        self.cache_size = cache_size
        self.count_cache_file = count_cache_file
        
        self.file_cache = {}
        self.cache_order = []
        
        if lazy_count:
            print("Using lazy counting mode (equal weights for file selection)")
            self.file_text_counts = [1] * len(self.dataset_paths)
            total_valid = len(self.dataset_paths)
        else:
            print("Counting texts in all files (this may take a while for large files)...")
            self.file_text_counts = []
            total_valid = 0
            
            cached_counts = self._load_count_cache()
            
            for path in self.dataset_paths:
                if path in cached_counts:
                    count = cached_counts[path]
                    print(f"File: {path} - {count} valid texts (cached)")
                else:
                    count = self._count_valid_texts_streaming(path)
                    print(f"File: {path} - {count} valid texts")
                    cached_counts[path] = count
                
                self.file_text_counts.append(count)
                total_valid += count
            
            self._save_count_cache(cached_counts)
            
            if total_valid == 0:
                raise ValueError("No valid texts found in any dataset file")
        
        self.file_weights = [count / total_valid for count in self.file_text_counts]
        
        print(f"\nVocabulary size: {self.vocab_size}")
        print(f"Cache size: {cache_size} file(s)")
        print(f"Number of dataset files: {len(self.dataset_paths)}")
    
    def _load_count_cache(self):
        if os.path.exists(self.count_cache_file):
            try:
                with open(self.count_cache_file, 'r') as f:
                    return json.load(f)
            except Exception:
                return {}
        return {}
    
    def _save_count_cache(self, counts):
        try:
            with open(self.count_cache_file, 'w') as f:
                json.dump(counts, f, indent=2)
        except Exception as e:
            print(f"Warning: Could not save count cache: {e}")
    
    def _count_valid_texts_streaming(self, file_path):
        if HAS_IJSON:
            count = 0
            try:
                with open(file_path, 'rb') as f:
                    for item in ijson.items(f, 'item'):
                        if 'tokenizedText' in item and len(item['tokenizedText']) > 1:
                            count += 1
                return count
            except Exception as e:
                print(f"Warning: Streaming parse failed ({e}), falling back to standard method")
        
        return self._count_valid_texts_fallback(file_path)
    
    def _count_valid_texts_fallback(self, file_path):
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
            mask_batch = []
            
            while len(X_batch) < batch_size:
                tokenized = self._get_random_text()
                
                if tokenized is None:
                    continue
                
                input_seq, target_seq = self._prepare_sequence(tokenized, max_sample_length)
                
                if input_seq is None:
                    continue
                
                mask = np.array([0.0 if token == 0 else 1.0 for token in target_seq], dtype=np.float32)
                
                X_batch.append(input_seq)
                Y_batch.append(target_seq)
                mask_batch.append(mask)
            
            yield (
                np.array(X_batch, dtype=np.int32), 
                np.array(Y_batch, dtype=np.int32),
                np.array(mask_batch, dtype=np.float32)
            )

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


class GoDataGenerator:
    def __init__(self, vocab_size, go_server_url="http://localhost:4567"):
        self.vocab_size = vocab_size
        self.go_server_url = go_server_url
        self.session = requests.Session()
        
        print(f"GoDataGenerator initialized with vocab_size: {vocab_size}")
        print(f"Go server URL: {go_server_url}")
        
        try:
            health_response = self.session.get(f"{go_server_url}/health", timeout=5)
            if health_response.status_code == 200:
                print("Go server is healthy and ready!")
            else:
                print(f"Warning: Go server health check returned status {health_response.status_code}")
        except Exception as e:
            print(f"Warning: Could not connect to Go server: {e}")
    
    def generate_batches(self, batch_size, max_sample_length):
        while True:
            try:
                response = self.session.post(
                    f"{self.go_server_url}/get-batch",
                    json={
                        "batch_size": batch_size,
                        "max_sample_length": max_sample_length
                    },
                    timeout=30
                )
                
                if response.status_code != 200:
                    print(f"Error from Go server: {response.status_code} - {response.text}")
                    continue
                
                data = response.json()
                samples = data["samples"]
                total_tokens = data["total_tokens"]
                
                X_batch = np.array([s["input_seq"] for s in samples], dtype=np.int32)
                Y_batch = np.array([s["target_seq"] for s in samples], dtype=np.int32)
                mask_batch = np.array([s["mask"] for s in samples], dtype=np.float32)
                
                yield (X_batch, Y_batch, mask_batch, total_tokens)
                
            except requests.exceptions.RequestException as e:
                print(f"Request error: {e}")
                import time
                time.sleep(1)
                continue
            except Exception as e:
                print(f"Unexpected error: {e}")
                import time
                time.sleep(1)
                continue


# Example usage:
if __name__ == "__main__":
    # Option 1: Lazy count with cache (FASTEST, LEAST MEMORY) - RECOMMENDED for large files
    data_gen = DataGenerator(
        ['extractedTexts_multilang.json', 'extractedTexts.json'],
        'tokenizer.json',
        cache_size=1,
        lazy_count=True
    )
    
    # Option 2: Count with streaming (slower init, accurate weights, moderate memory)
    # Requires: pip install ijson
    # data_gen = DataGenerator(
    #     ['extractedTexts_multilang.json', 'extractedTexts.json'],
    #     'tokenizer.json',
    #     cache_size=1,
    #     lazy_count=False
    # )
    
    # Option 3: Cache multiple files (faster sampling, more memory during training)
    # data_gen = DataGenerator(
    #     ['extractedTexts_multilang.json', 'extractedTexts.json'],
    #     'tokenizer.json',
    #     cache_size=2,
    #     lazy_count=True
    # )
    
    # Option 4: Preload all files (fastest sampling, most memory)
    # data_gen = DataGenerator(
    #     ['extractedTexts_multilang.json', 'extractedTexts.json'],
    #     'tokenizer.json',
    #     lazy_count=True
    # )
    # data_gen.preload_all_files()
    
    print("\n=== Testing generate_batches ===")
    for i, (X, Y, mask) in enumerate(data_gen.generate_batches(batch_size=2, max_sample_length=10)):
        print(f"\nBatch {i+1}:")
        print("Input batch shape:", X.shape)
        print("Target batch shape:", Y.shape)
        print("Mask batch shape:", mask.shape)
        print("Example input sequence:", X[0])
        print("Example target sequence:", Y[0])
        print("Example mask:", mask[0])
        print("Non-padding tokens:", int(mask[0].sum()))
        
        if i >= 2:
            break