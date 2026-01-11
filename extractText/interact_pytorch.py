import os
import torch
import torch.nn as nn
import torch.nn.functional as F
import numpy as np
from tokenizers import Tokenizer
from train_pytorch import CausalLanguageModel, FLASH_AVAILABLE
import time


class FastTextGenerator:
    def __init__(self, weights_path, tokenizer_path, context_window=2048, d_model=2048, num_heads=32, num_layers=18, num_kv_heads=8, dropout_rate=0.0, use_compile=True):
        self.device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
        self.dtype = torch.bfloat16 if torch.cuda.is_available() else torch.float32
        
        print(f"Using device: {self.device}")
        print(f"Using dtype: {self.dtype}")
        
        self.tokenizer = Tokenizer.from_file(tokenizer_path)
        self.vocab_size = self.tokenizer.get_vocab_size()
        self.context_window = context_window
        
        print(f"Building PyTorch model architecture...")
        self.model = CausalLanguageModel(
            vocab_size=self.vocab_size,
            context_window=context_window,
            d_model=d_model,
            num_heads=num_heads,
            num_layers=num_layers,
            ff_dim=int(d_model * 8 / 3),
            dropout=dropout_rate,
            num_kv_heads=num_kv_heads
        )
        
        print(f"Loading weights from {weights_path}...")
        checkpoint = torch.load(weights_path, map_location=self.device, weights_only=False)
        state_dict = checkpoint.get('model_state_dict', checkpoint)
        state_dict = {k.replace('_orig_mod.', ''): v for k, v in state_dict.items()}
        self.model.load_state_dict(state_dict, strict=False)
        
        self.model.to(device=self.device, dtype=self.dtype)
        self.model.eval()
        
        total_params = sum(p.numel() for p in self.model.parameters() if p.requires_grad)
        print(f"Model loaded! Total parameters: {total_params:,}")
        print(f"Model size: ~{total_params * 2 / 1e9:.2f} GB (bfloat16)")
        
        if use_compile and torch.cuda.is_available():
            print("\nCompiling model with torch.compile for faster inference...")
            print("This will take 1-2 minutes on first run, then inference will be much faster.")
            self.model = torch.compile(self.model, mode="reduce-overhead")
        
        print("\nWarming up model...")
        dummy_input = torch.randint(0, self.vocab_size, (1, 10), device=self.device)
        with torch.no_grad():
            _ = self.model(dummy_input)
        print("Model ready for fast inference!\n")
    
    @torch.inference_mode()
    def generate(self, prompt, max_length=256, temperature=0.8, top_k=40, top_p=0.9, repetition_penalty=1.2):
        tokens = self.tokenizer.encode(prompt).ids
        
        if len(tokens) > self.context_window:
            tokens = tokens[-self.context_window:]
        
        print(f"Starting generation (max {max_length} tokens)...")
        start_time = time.time()
        
        generated_tokens = 0
        
        for i in range(max_length):
            input_seq = tokens[-self.context_window:]
            input_tensor = torch.tensor([input_seq], dtype=torch.long, device=self.device)
            
            with torch.amp.autocast('cuda', dtype=self.dtype):
                logits = self.model(input_tensor)
                next_token_logits = logits[0, -1, :]
            
            if repetition_penalty != 1.0:
                for token_id in set(tokens):
                    if next_token_logits[token_id] < 0:
                        next_token_logits[token_id] *= repetition_penalty
                    else:
                        next_token_logits[token_id] /= repetition_penalty
            
            if temperature > 0:
                next_token_logits = next_token_logits / temperature
                
                if top_k > 0:
                    top_k_values, top_k_indices = torch.topk(next_token_logits, min(top_k, next_token_logits.size(-1)))
                    next_token_logits = torch.full_like(next_token_logits, float('-inf'))
                    next_token_logits.scatter_(0, top_k_indices, top_k_values)
                
                probs = F.softmax(next_token_logits, dim=-1)
                
                if top_p < 1.0:
                    sorted_probs, sorted_indices = torch.sort(probs, descending=True)
                    cumulative_probs = torch.cumsum(sorted_probs, dim=-1)
                    sorted_indices_to_remove = cumulative_probs > top_p
                    sorted_indices_to_remove[1:] = sorted_indices_to_remove[:-1].clone()
                    sorted_indices_to_remove[0] = False
                    indices_to_remove = sorted_indices[sorted_indices_to_remove]
                    probs[indices_to_remove] = 0.0
                    probs = probs / probs.sum()
                
                next_token = torch.multinomial(probs, num_samples=1).item()
            else:
                next_token = torch.argmax(next_token_logits).item()
            
            tokens.append(next_token)
            generated_tokens += 1
            
            decoded = self.tokenizer.decode([next_token])
            if '<EOS>' in decoded or decoded.strip() in ['</s>', '<|endoftext|>']:
                break
        
        elapsed = time.time() - start_time
        tokens_per_sec = generated_tokens / elapsed if elapsed > 0 else 0
        
        return self.tokenizer.decode(tokens), generated_tokens, tokens_per_sec
    
    @torch.inference_mode()
    def generate_streaming(self, prompt, max_length=256, temperature=0.8, top_k=40, top_p=0.9, repetition_penalty=1.2):
        tokens = self.tokenizer.encode(prompt).ids
        
        if len(tokens) > self.context_window:
            tokens = tokens[-self.context_window:]
        
        for i in range(max_length):
            input_seq = tokens[-self.context_window:]
            input_tensor = torch.tensor([input_seq], dtype=torch.long, device=self.device)
            
            with torch.amp.autocast('cuda', dtype=self.dtype):
                logits = self.model(input_tensor)
                next_token_logits = logits[0, -1, :]
            
            if repetition_penalty != 1.0:
                for token_id in set(tokens):
                    if next_token_logits[token_id] < 0:
                        next_token_logits[token_id] *= repetition_penalty
                    else:
                        next_token_logits[token_id] /= repetition_penalty
            
            if temperature > 0:
                next_token_logits = next_token_logits / temperature
                
                if top_k > 0:
                    top_k_values, top_k_indices = torch.topk(next_token_logits, min(top_k, next_token_logits.size(-1)))
                    next_token_logits = torch.full_like(next_token_logits, float('-inf'))
                    next_token_logits.scatter_(0, top_k_indices, top_k_values)
                
                probs = F.softmax(next_token_logits, dim=-1)
                
                if top_p < 1.0:
                    sorted_probs, sorted_indices = torch.sort(probs, descending=True)
                    cumulative_probs = torch.cumsum(sorted_probs, dim=-1)
                    sorted_indices_to_remove = cumulative_probs > top_p
                    sorted_indices_to_remove[1:] = sorted_indices_to_remove[:-1].clone()
                    sorted_indices_to_remove[0] = False
                    indices_to_remove = sorted_indices[sorted_indices_to_remove]
                    probs[indices_to_remove] = 0.0
                    probs = probs / probs.sum()
                
                next_token = torch.multinomial(probs, num_samples=1).item()
            else:
                next_token = torch.argmax(next_token_logits).item()
            
            tokens.append(next_token)
            
            decoded = self.tokenizer.decode([next_token])
            if '<EOS>' in decoded or decoded.strip() in ['</s>', '<|endoftext|>']:
                break
            
            yield decoded
    
    @torch.inference_mode()
    def compute_perplexity(self, text):
        tokens = self.tokenizer.encode(text).ids
        
        if len(tokens) < 2:
            return float('inf')
        
        input_tensor = torch.tensor([tokens[:-1]], dtype=torch.long, device=self.device)
        target_tensor = torch.tensor([tokens[1:]], dtype=torch.long, device=self.device)
        
        with torch.amp.autocast('cuda', dtype=self.dtype):
            logits = self.model(input_tensor)
        
        loss = F.cross_entropy(logits[0], target_tensor[0], reduction='mean')
        perplexity = torch.exp(loss).item()
        
        return perplexity


def main():
    print("Loading fast PyTorch model...")
    generator = FastTextGenerator(
        weights_path='best_model_pytorch_v2.pt',
        tokenizer_path='tokenizer.json',
        context_window=2048,
        d_model=2048,
        num_heads=32,
        num_layers=18,
        num_kv_heads=8,
        dropout_rate=0.0,
        use_compile=True
    )
    print("Model loaded successfully\n")
    
    print("Commands:")
    print("  Type your prompt and press Enter")
    print("  Type 'quit' or 'exit' to stop")
    print("  Type 'clear' to clear screen")
    print("  Type 'stream' to toggle streaming mode")
    print("  Type 'perplexity <text>' to compute perplexity")
    print("  Type 'temp <value>' to set temperature (0.0-2.0)")
    print("  Type 'length <value>' to set max length (1-2048)")
    print("  Type 'topk <value>' to set top-k (1-100)")
    print("  Type 'topp <value>' to set top-p (0.0-1.0)")
    print("  Type 'penalty <value>' to set repetition penalty (1.0-2.0)")
    print("  Type 'settings' to show current settings\n")
    
    streaming_mode = False
    temperature = 0.3
    max_length = 256
    top_k = 20
    top_p = 0.85
    repetition_penalty = 1.75
    
    while True:
        try:
            prompt = input("\n### Prompt:\n").strip()
            
            if prompt.lower() in ['quit', 'exit']:
                print("Goodbye!")
                break
                
            if prompt.lower() == 'clear':
                print("\n" * 50)
                continue
            
            if prompt.lower() == 'stream':
                streaming_mode = not streaming_mode
                print(f"Streaming mode: {'ON' if streaming_mode else 'OFF'}")
                continue
            
            if prompt.lower() == 'settings':
                print(f"\nCurrent settings:")
                print(f"  Temperature: {temperature}")
                print(f"  Max length: {max_length}")
                print(f"  Top-k: {top_k}")
                print(f"  Top-p: {top_p}")
                print(f"  Repetition penalty: {repetition_penalty}")
                print(f"  Streaming: {'ON' if streaming_mode else 'OFF'}")
                continue
            
            if prompt.lower().startswith('temp '):
                try:
                    temperature = float(prompt.split()[1])
                    temperature = max(0.0, min(2.0, temperature))
                    print(f"Temperature set to: {temperature}")
                except:
                    print("Usage: temp <value> (0.0-2.0)")
                continue
            
            if prompt.lower().startswith('length '):
                try:
                    max_length = int(prompt.split()[1])
                    max_length = max(1, min(2048, max_length))
                    print(f"Max length set to: {max_length}")
                except:
                    print("Usage: length <value> (1-2048)")
                continue
            
            if prompt.lower().startswith('topk '):
                try:
                    top_k = int(prompt.split()[1])
                    top_k = max(1, min(100, top_k))
                    print(f"Top-k set to: {top_k}")
                except:
                    print("Usage: topk <value> (1-100)")
                continue
            
            if prompt.lower().startswith('topp '):
                try:
                    top_p = float(prompt.split()[1])
                    top_p = max(0.0, min(1.0, top_p))
                    print(f"Top-p set to: {top_p}")
                except:
                    print("Usage: topp <value> (0.0-1.0)")
                continue
            
            if prompt.lower().startswith('penalty '):
                try:
                    repetition_penalty = float(prompt.split()[1])
                    repetition_penalty = max(1.0, min(2.0, repetition_penalty))
                    print(f"Repetition penalty set to: {repetition_penalty}")
                except:
                    print("Usage: penalty <value> (1.0-2.0, 1.0=off)")
                continue
            
            if prompt.lower().startswith('perplexity '):
                text = prompt[11:]
                if text:
                    perplexity = generator.compute_perplexity(text)
                    print(f"\nPerplexity: {perplexity:.2f}")
                continue
                
            if not prompt:
                continue
            
            formatted_prompt = f"### Prompt:\n{prompt}\n### Response:\n"
            
            if streaming_mode:
                print("### Response:\n", end='', flush=True)
                start_time = time.time()
                token_count = 0

                for token_text in generator.generate_streaming(formatted_prompt, max_length=max_length, temperature=temperature, top_k=top_k, top_p=top_p, repetition_penalty=repetition_penalty):
                    print(token_text, end=' ', flush=True)
                    token_count += 1
                elapsed = time.time() - start_time
                tokens_per_sec = token_count / elapsed if elapsed > 0 else 0
                print(f"\n\n[Generated {token_count} tokens in {elapsed:.2f}s ({tokens_per_sec:.1f} tokens/sec)]")
            else:
                response, num_tokens, tokens_per_sec = generator.generate(
                    formatted_prompt, 
                    max_length=max_length, 
                    temperature=temperature, 
                    top_k=top_k,
                    top_p=top_p,
                    repetition_penalty=repetition_penalty
                )
                
                print("### Response:")
                print(response)
                print(f"\n[Generated {num_tokens} tokens ({tokens_per_sec:.1f} tokens/sec)]")
                
                print("\nAssistant:", response)
                print(f"\n[Generated {num_tokens} tokens ({tokens_per_sec:.1f} tokens/sec)]")
            
        except KeyboardInterrupt:
            print("\n\nGoodbye!")
            break
        except Exception as e:
            print(f"\nError: {e}")
            import traceback
            traceback.print_exc()


if __name__ == '__main__':
    main()
