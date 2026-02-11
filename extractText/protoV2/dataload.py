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

def load_streaming_dataset(name, split=None, subset=None):
    try:
        if subset and split:
            return load_dataset(name, subset, split=split, streaming=True)
        if subset:
            return load_dataset(name, subset, streaming=True)
        if split:
            return load_dataset(name, split=split, streaming=True)
        return load_dataset(name, streaming=True)
    except Exception:
        if subset:
            dataset = load_dataset(name, subset)
        else:
            dataset = load_dataset(name)
        if split and split in dataset:
            return dataset[split]
        return dataset[list(dataset.keys())[0]]

def iter_nextcoder():
    dataset = load_streaming_dataset("microsoft/NextCoderDataset", split="train")
    for item in iter(dataset):
        prompt = item.get('prompt', '')
        completion = item.get('completion', '')
        combined_text = f"{prompt}\n{completion}"
        if combined_text.strip():
            yield combined_text

def iter_openthoughts():
    dataset = load_streaming_dataset("open-thoughts/OpenThoughts3-1.2M", split="train")
    for item in iter(dataset):
        conversations = item.get('conversations') or []
        if not conversations:
            continue
        parts = []
        for turn in conversations:
            role = (turn.get('from') or '').capitalize()
            if role == 'Gpt':
                role = 'Assistant'
            value = turn.get('value', '')
            if value:
                parts.append(f"{role}:\n{value}")
        combined_text = "\n".join(parts)
        if combined_text.strip():
            yield combined_text

def iter_codeforces(subset):
    dataset = load_streaming_dataset("open-r1/codeforces-cots", subset=subset, split="train")
    for item in iter(dataset):
        messages = item.get('messages') or []
        parts = []
        for msg in messages:
            role = msg.get('role', '')
            if role == 'user':
                label = 'User'
            elif role == 'assistant':
                label = 'Assistant'
            else:
                label = role.capitalize() if role else 'Unknown'
            content = msg.get('content', '')
            if content:
                parts.append(f"{label}:\n{content}")
        combined_text = "\n".join(parts)
        if combined_text.strip():
            yield combined_text

def iter_openmath():
    dataset = load_streaming_dataset("nvidia/OpenMathReasoning", split="train")
    for item in iter(dataset):
        problem = item.get('problem', '')
        solution = item.get('generated_solution', '')
        combined_text = f"{problem}\n{solution}"
        if combined_text.strip():
            yield combined_text

def iter_leetcode():
    dataset = load_streaming_dataset("newfacade/LeetCodeDataset", split="train")
    for item in iter(dataset):
        query = item.get('query', '')
        response = item.get('response', '')
        combined_text = f"{query}\n{response}"
        if combined_text.strip():
            yield combined_text

def process_training_sample(min_words=50, max_words=500):
    import random
    dataset_iters = [
        iter_nextcoder(),
        iter_openthoughts(),
        iter_codeforces("solutions_py"),
        iter_codeforces("solutions"),
        iter_openmath(),
        iter_leetcode()
    ]
    dataset_index = 0

    while dataset_iters:
        if dataset_index >= len(dataset_iters):
            dataset_index = 0
        iterator = dataset_iters[dataset_index]
        try:
            combined_text = next(iterator)
            dataset_index += 1
        except StopIteration:
            dataset_iters.pop(dataset_index)
            continue
        try:
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
