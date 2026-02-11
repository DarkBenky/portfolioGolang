import torch
from transformers import AutoTokenizer, AutoModel
from datasets import load_dataset

MODEL_NAME = "Qwen/Qwen2-1.5B"

tokenizer = AutoTokenizer.from_pretrained(MODEL_NAME)
EMBED_DEVICE = torch.device("cuda" if torch.cuda.is_available() else "cpu")
model = AutoModel.from_pretrained(MODEL_NAME)
model.eval()
model.to(EMBED_DEVICE)
embed_layer = model.get_input_embeddings()
EMBED_DIM = embed_layer.embedding_dim
PAD_ID = tokenizer.pad_token_id or 0

def get_embedding_weight():
    return embed_layer.weight.detach()

def text_to_padded_tokens(text, window_size):
    tokens = tokenizer(text, return_tensors="pt")["input_ids"][0]

    tokens = tokens[:window_size]

    seq_len = tokens.shape[0]

    if seq_len < window_size:
        pad = torch.full(
            (window_size - seq_len,),
            PAD_ID,
            dtype=torch.long,
            device=tokens.device
        )
        tokens = torch.cat([tokens, pad], dim=0)

    return tokens

def build_training_pair(text, window_size):
    tokens = text_to_padded_tokens(text, window_size)

    target_tokens = tokens.clone()
    target_tokens[:-1] = tokens[1:]
    target_tokens[-1] = PAD_ID

    return tokens, target_tokens

def process_nextcoder_sample(min_words=50, max_words=500):
    import random
    
    dataset = load_dataset("microsoft/NextCoderDataset", split="train", streaming=True)
    
    for item in iter(dataset):
        try:
            prompt = item.get('prompt', '')
            completion = item.get('completion', '')
            combined_text = f"{prompt}\n{completion}"
            
            words = combined_text.split()
            total_words = len(words)
            
            if total_words < min_words:
                continue
            
            sample_length = random.randint(min_words, min(max_words, total_words))
            
            if total_words > sample_length:
                max_start = total_words - sample_length
                start_idx = random.randint(0, max_start)
                sampled_words = words[start_idx:start_idx + sample_length]
            else:
                sampled_words = words
            
            sampled_text = ' '.join(sampled_words)
            if tokenizer.eos_token:
                sampled_text = sampled_text + tokenizer.eos_token
            
            yield sampled_text
        
        except Exception as e:
            print(f"Error processing item: {e}")
            continue
