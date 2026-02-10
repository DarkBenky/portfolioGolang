import torch
from transformers import AutoTokenizer, AutoModel
from datasets import load_dataset

from extractText.saveDataToRag import load_progress

MODEL_NAME = "meta-llama/Llama-2-7b-hf"

tokenizer = AutoTokenizer.from_pretrained(MODEL_NAME)
model = AutoModel.from_pretrained(MODEL_NAME)

embed_layer = model.model.embed_tokens
PAD_ID = tokenizer.pad_token_id or 0

def text_to_padded_embeddings(text, window_size):
    tokens = tokenizer(text, return_tensors="pt")["input_ids"][0]

    # Truncate if too long
    tokens = tokens[:window_size]

    seq_len = tokens.shape[0]

    # Pad if too short
    if seq_len < window_size:
        pad = torch.full(
            (window_size - seq_len,),
            PAD_ID,
            dtype=torch.long
        )
        tokens = torch.cat([tokens, pad], dim=0)

    # Convert to embeddings
    with torch.no_grad():
        embeds = embed_layer(tokens)

    return embeds, tokens

def build_training_pair(text, window_size):
    embeds, tokens = text_to_padded_embeddings(text, window_size)

    target_tokens = tokens.clone()

    target_tokens[:-1] = tokens[1:]
    target_tokens[-1] = PAD_ID

    return embeds, target_tokens

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
            
            yield sampled_text
        
        except Exception as e:
            print(f"Error processing item: {e}")
            continue