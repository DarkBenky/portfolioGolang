from tokenizers import Tokenizer
from specialCharacters import END_TOKEN, GENERIC_TEXTS, MATH_TEXTS, CODE_MIX_TEXTS, OPEN_THOUGHTS_TEXTS, FINE_BERT_TEXTS, SCIENCE_TEXTS
from sentence_transformers import SentenceTransformer
import chromadb
import uuid
import requests
import os
import torch
import gc
from pprint import pprint

os.environ['HF_HOME'] = '/media/user/free/data'
os.environ['HF_DATASETS_CACHE'] = '/media/user/free/data/huggingface_cache'
os.environ['PYTORCH_CUDA_ALLOC_CONF'] = 'expandable_segments:True'

from datasets import load_dataset

# Init embedding model - Qwen3-Embedding-0.6B is optimized for code, math, and multilingual tasks
# It ranks #1 on MTEB-Code benchmark and supports 100+ languages with 32K context length
embedder = SentenceTransformer("Qwen/Qwen3-Embedding-0.6B")

# For even better performance (but slower), you can use:
# embedder = SentenceTransformer("Qwen/Qwen3-Embedding-4B")  # Higher accuracy
# or
# embedder = SentenceTransformer("BAAI/bge-large-en-v1.5")  # Good general-purpose alternative

# Init Chroma with PersistentClient
persist_dir = "/media/user/free/data/ragDb"
client = chromadb.PersistentClient(path=persist_dir)

collection_name = "rag_data"
collection = client.get_or_create_collection(
    name=collection_name
)

def chunk_text(
    text: str,
    chunk_size: int = 512,
    overlap: int = 64,
):
    chunks = []
    start = 0
    text_len = len(text)

    while start < text_len:
        end = start + chunk_size
        chunk = text[start:end]
        chunks.append(chunk)
        start = end - overlap

        if start < 0:
            start = 0

    return chunks

def embed_text_to_rag(
    text: str,
    chunk_size: int = 512,
    overlap: int = 64,
):
    # Split text into chunks
    chunks = chunk_text(
        text,
        chunk_size=chunk_size,
        overlap=overlap,
    )

    embeddings = embedder.encode(
        chunks,
        batch_size=8,
        normalize_embeddings=True,
        show_progress_bar=False
    )

    ids = [str(uuid.uuid4()) for _ in chunks]

    collection.add(
        documents=chunks,
        embeddings=embeddings.tolist(),
        ids=ids,
    )

    torch.cuda.empty_cache()
    gc.collect()

    return {
        "chunks_added": len(chunks),
        "collection": collection_name,
    }

tokenizer = Tokenizer.from_file('tokenizer.json')
print("Vocabulary size:", tokenizer.get_vocab_size())

def format_tokens(count: int) -> str:
    if count >= 1_000_000_000_000:
        return f"{count / 1_000_000_000_000:.2f}T"
    elif count >= 1_000_000_000:
        return f"{count / 1_000_000_000:.2f}B"
    elif count >= 1_000_000:
        return f"{count / 1_000_000:.2f}M"
    elif count >= 1_000:
        return f"{count / 1_000:.2f}K"
    else:
        return str(count)

def remove_think_tags(text: str) -> str:
    if "<think>" in text and "</think>" in text:
        start_think = text.index("<think>")
        end_think = text.index("</think>") + len("</think>")
        return text[:start_think] + text[end_think:]
    return text

def query_rag(
    query: str,
    n_results: int = 5,
):
    # Qwen3-Embedding doesn't need query prefixes
    # The model is trained to understand queries naturally
    query_embedding = embedder.encode(
        [query], 
        normalize_embeddings=True,
        show_progress_bar=False
    )
    
    results = collection.query(
        query_embeddings=query_embedding.tolist(),
        n_results=n_results
    )
    
    return {
        "query": query,
        "results": results['documents'][0] if results['documents'] else [],
        "distances": results['distances'][0] if results['distances'] else [],
        "ids": results['ids'][0] if results['ids'] else []
    }

def process_am_deepseek_distilled_40m():
    splits = [
        ['code_1.5b_1pass', CODE_MIX_TEXTS, 7615],
        ['code_1.5b_2pass', CODE_MIX_TEXTS, 0],
        ['code_1.5b_3pass', CODE_MIX_TEXTS, 0],
        ['code_1.5b_4pass', CODE_MIX_TEXTS, 0],
        ['code_7b_1pass', CODE_MIX_TEXTS, 0],
        ['code_7b_2pass', CODE_MIX_TEXTS, 0],
        ['code_7b_3pass', CODE_MIX_TEXTS, 0],
        ['code_7b_4pass', CODE_MIX_TEXTS, 0],
        ['code_r1_1pass', CODE_MIX_TEXTS, 0],
        ['code_r1_2pass', CODE_MIX_TEXTS, 0],
        ['code_r1_3pass', CODE_MIX_TEXTS, 0],
        ['code_r1_4pass', CODE_MIX_TEXTS, 0],
        ['if_1.5b_1pass', GENERIC_TEXTS, 0],
        ['if_1.5b_2pass', GENERIC_TEXTS, 0],
        ['if_1.5b_3pass', GENERIC_TEXTS, 0],
        ['if_1.5b_4pass', GENERIC_TEXTS, 0],
        ['if_7b_1pass', GENERIC_TEXTS, 0],
        ['if_7b_2pass', GENERIC_TEXTS, 0],
        ['if_7b_3pass', GENERIC_TEXTS, 0],
        ['if_7b_4pass', GENERIC_TEXTS, 0],
        ['if_r1_1pass', GENERIC_TEXTS, 0],
        ['if_r1_2pass', GENERIC_TEXTS, 0],
        ['if_r1_3pass', GENERIC_TEXTS, 0],
        ['if_r1_4pass', GENERIC_TEXTS, 0],
        ['math_1.5b_1pass', MATH_TEXTS, 0],
        ['math_1.5b_2pass', MATH_TEXTS, 0],
        ['math_1.5b_3pass', MATH_TEXTS, 0],
        ['math_1.5b_4pass', MATH_TEXTS, 0],
        ['math_7b_1pass', MATH_TEXTS, 0],
        ['math_7b_2pass', MATH_TEXTS, 0],
        ['math_7b_3pass', MATH_TEXTS, 0],
        ['math_7b_4pass', MATH_TEXTS, 0],
        ['math_r1_1pass', MATH_TEXTS, 0],
        ['math_r1_2pass', MATH_TEXTS, 0],
        ['math_r1_3pass', MATH_TEXTS, 0],
        ['math_r1_4pass', MATH_TEXTS, 0],
    ]
    
    for split in splits:
        split_name = split[0]
        category = split[1]
        offset = split[2]

        dataset = load_dataset("a-m-team/AM-DeepSeek-Distilled-40M", name=split_name, split="train", streaming=True)

        count = 0
        for item in iter(dataset):
            count += 1
            if count <= offset:
                continue

            question = item['question']
            answer = item['answer'] + END_TOKEN
            combined_text = f"### User:\n{question}\n\n### Assistant:\n{answer}"

            combined_text_rag = remove_think_tags(combined_text)

            result = embed_text_to_rag(
                combined_text_rag,
                chunk_size=1024,
                overlap=128,
            )

            try:
                data = {
                    'text': combined_text,
                    'tokenized_text': tokenizer.encode(combined_text).ids,
                    'category': category
                }
                url = "http://localhost:4567/save-data"
                response = requests.post(url, json=data)
                if response.status_code != 200:
                    print(f"Failed to save data: {response.text}")
                else:
                    result = response.json()
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
            except Exception as e:
                print(f"Error saving data: {e}")

if __name__ == "__main__":
    process_am_deepseek_distilled_40m()