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
import json
from concurrent.futures import ThreadPoolExecutor, as_completed, ProcessPoolExecutor
import threading
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry
import wandb
import time

os.environ['HF_HOME'] = '/media/user/2TB'
os.environ['HF_DATASETS_CACHE'] = '/media/user/2TB/huggingface_cache'
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
persist_dir = "/media/user/2TB/tokenizedData/ragDb"
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
        batch_size=1,
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

session = requests.Session()
retry_strategy = Retry(
    total=3,
    backoff_factor=0.1,
    status_forcelist=[429, 500, 502, 503, 504],
)
adapter = HTTPAdapter(max_retries=retry_strategy, pool_connections=10, pool_maxsize=20)
session.mount("http://", adapter)
session.mount("https://", adapter)

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

PROGRESS_FILE = "dataset_progress.json"
progress_lock = threading.Lock()

def load_progress():
    with progress_lock:
        if os.path.exists(PROGRESS_FILE):
            try:
                with open(PROGRESS_FILE, 'r') as f:
                    return json.load(f)
            except Exception as e:
                print(f"Error loading progress file: {e}")
                return {}
        return {}

def save_progress(progress_data):
    with progress_lock:
        try:
            with open(PROGRESS_FILE, 'w') as f:
                json.dump(progress_data, f, indent=2)
        except Exception as e:
            print(f"Error saving progress: {e}")

def update_progress(key, value):
    with progress_lock:
        try:
            current_data = {}
            if os.path.exists(PROGRESS_FILE):
                with open(PROGRESS_FILE, 'r') as f:
                    current_data = json.load(f)
            current_data[key] = value
            with open(PROGRESS_FILE, 'w') as f:
                json.dump(current_data, f, indent=2)
        except Exception as e:
            print(f"Error updating progress: {e}")

def query_rag(
    query: str,
    n_results: int = 5,
):
    # Qwen3-Embedding doesn't need query prefixes
    # The model is trained to understand queries naturally
    query_embedding = embedder.encode(
        [query], 
        batch_size=1,
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

def process_am_deepseek_split(split_name, category):
    SAVE_PERIOD_TO_RAG = 128
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get(split_name, 0)

    print(f"\n{'='*60}")
    print(f"Processing AM-DeepSeek split: {split_name}")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")

    dataset = load_dataset("a-m-team/AM-DeepSeek-Distilled-40M", name=split_name, split="train", streaming=True)

    count = 0
    for item in iter(dataset):
        count += 1
        if count <= offset:
            continue

        question = item['question']
        answer = item['answer'] + END_TOKEN
        combined_text = f"### Prompt:\n{question}\n### Response:\n{answer}"

        try:
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': category
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"{split_name}/file_counter": result.get('file_counter', 0),
                        f"{split_name}/processed_tokens": result.get('processed_tokens', 0),
                        f"{split_name}/requests_per_sec": result.get('requests_per_sec', 0),
                        f"{split_name}/samples": count,
                    })
        except Exception as e:
            print(f"Error saving data: {e}")

        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress(split_name, count)
            print(f"Progress saved: {split_name} at {count}")

    update_progress(split_name, count)
    print(f"Split {split_name} completed at {count}")

def process_am_if_r1_2pass():
    process_am_deepseek_split('if_r1_2pass', GENERIC_TEXTS)

def process_am_if_r1_3pass():
    process_am_deepseek_split('if_r1_3pass', GENERIC_TEXTS)

def process_am_code_r1_2pass():
    process_am_deepseek_split('code_r1_2pass', CODE_MIX_TEXTS)

def process_am_code_r1_4pass():
    process_am_deepseek_split('code_r1_4pass', CODE_MIX_TEXTS)

def process_am_math_r1_2pass():
    process_am_deepseek_split('math_r1_2pass', MATH_TEXTS)

def process_am_math_r1_3pass():
    process_am_deepseek_split('math_r1_3pass', MATH_TEXTS)

def process_am_math_r1_4pass():
    process_am_deepseek_split('math_r1_4pass', MATH_TEXTS)

def process_am_code_r1_1pass():
    process_am_deepseek_split('code_r1_1pass', CODE_MIX_TEXTS)

def process_am_code_7b_1pass():
    process_am_deepseek_split('code_7b_1pass', CODE_MIX_TEXTS)

def process_am_code_7b_2pass():
    process_am_deepseek_split('code_7b_2pass', CODE_MIX_TEXTS)

def process_am_code_7b_3pass():
    process_am_deepseek_split('code_7b_3pass', CODE_MIX_TEXTS)

def process_am_code_7b_4pass():
    process_am_deepseek_split('code_7b_4pass', CODE_MIX_TEXTS)

def process_am_if_7b_1pass():
    process_am_deepseek_split('if_7b_1pass', GENERIC_TEXTS)

def process_am_if_7b_2pass():
    process_am_deepseek_split('if_7b_2pass', GENERIC_TEXTS)

def process_am_if_7b_3pass():
    process_am_deepseek_split('if_7b_3pass', GENERIC_TEXTS)

def process_am_if_7b_4pass():
    process_am_deepseek_split('if_7b_4pass', GENERIC_TEXTS)

def process_am_if_r1_1pass():
    process_am_deepseek_split('if_r1_1pass', GENERIC_TEXTS)

def process_am_if_r1_4pass():
    process_am_deepseek_split('if_r1_4pass', GENERIC_TEXTS)

def process_am_math_7b_1pass():
    process_am_deepseek_split('math_7b_1pass', MATH_TEXTS)

def process_am_math_7b_2pass():
    process_am_deepseek_split('math_7b_2pass', MATH_TEXTS)

def process_am_math_7b_3pass():
    process_am_deepseek_split('math_7b_3pass', MATH_TEXTS)

def process_am_math_7b_4pass():
    process_am_deepseek_split('math_7b_4pass', MATH_TEXTS)

def process_nextcoder():
    SAVE_PERIOD_TO_RAG = 16
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get('nextcoder', 0)
    
    print(f"\n{'='*60}")
    print(f"Processing: microsoft/NextCoderDataset")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    
    dataset = load_dataset("microsoft/NextCoderDataset", split="train", streaming=True)
    
    count = 0
    for item in iter(dataset):
        count += 1
        if count <= offset:
            continue
        
        try:
            prompt = item['prompt']
            completion = item['completion']
            combined_text = f"### Prompt:\n{prompt}\n### Response:\n{completion}{END_TOKEN}"
            
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"nextcoder/file_counter": result.get('file_counter', 0),
                        f"nextcoder/processed_tokens": result.get('processed_tokens', 0),
                        f"nextcoder/requests_per_sec": result.get('requests_per_sec', 0),
                        f"nextcoder/samples": count,
                    })
            
            if count % SAVE_PROGRESS_INTERVAL == 0:
                update_progress('nextcoder', count)
                print(f"Progress saved: nextcoder at {count}")
        
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
    
    update_progress('nextcoder', count)
    print(f"Dataset nextcoder completed at {count}")

def process_codeforces_py():
    SAVE_PERIOD_TO_RAG = 16
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get('codeforces_py', 0)
    
    print(f"\n{'='*60}")
    print(f"Processing: open-r1/codeforces-cots (solutions_py)")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    
    dataset = load_dataset("open-r1/codeforces-cots", "solutions_py", split="train", streaming=True)
    
    count = 0
    for item in iter(dataset):
        count += 1
        if count <= offset:
            continue
        
        try:
            messages = item['messages']
            combined_text = ""
            for msg in messages:
                if msg['role'] == 'user':
                    combined_text += f"### Prompt:\n{msg['content']}\n"
                elif msg['role'] == 'assistant':
                    combined_text += f"### Response:\n{msg['content']}\n"
            combined_text += END_TOKEN
            
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"codeforces_py/file_counter": result.get('file_counter', 0),
                        f"codeforces_py/processed_tokens": result.get('processed_tokens', 0),
                        f"codeforces_py/requests_per_sec": result.get('requests_per_sec', 0),
                        f"codeforces_py/samples": count,
                    })
            
            if count % SAVE_PROGRESS_INTERVAL == 0:
                update_progress('codeforces_py', count)
                print(f"Progress saved: codeforces_py at {count}")
        
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
    
    update_progress('codeforces_py', count)
    print(f"Dataset codeforces_py completed at {count}")

def process_codeforces():
    SAVE_PERIOD_TO_RAG = 16
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get('codeforces', 0)
    
    print(f"\n{'='*60}")
    print(f"Processing: open-r1/codeforces-cots (solutions)")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    
    dataset = load_dataset("open-r1/codeforces-cots", "solutions", split="train", streaming=True)
    
    count = 0
    for item in iter(dataset):
        count += 1
        if count <= offset:
            continue
        
        try:
            messages = item['messages']
            combined_text = ""
            for msg in messages:
                if msg['role'] == 'user':
                    combined_text += f"### Prompt:\n{msg['content']}\n"
                elif msg['role'] == 'assistant':
                    combined_text += f"### Response:\n{msg['content']}\n"
            combined_text += END_TOKEN
            
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"codeforces/file_counter": result.get('file_counter', 0),
                        f"codeforces/processed_tokens": result.get('processed_tokens', 0),
                        f"codeforces/requests_per_sec": result.get('requests_per_sec', 0),
                        f"codeforces/samples": count,
                    })
            
            if count % SAVE_PROGRESS_INTERVAL == 0:
                update_progress('codeforces', count)
                print(f"Progress saved: codeforces at {count}")
        
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
    
    update_progress('codeforces', count)
    print(f"Dataset codeforces completed at {count}")

def process_opc_sft():
    SAVE_PERIOD_TO_RAG = 16
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get('opc_sft', 0)
    
    print(f"\n{'='*60}")
    print(f"Processing: OpenCoder-LLM/opc-sft-stage2 (educational_instruct)")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    
    dataset = load_dataset("OpenCoder-LLM/opc-sft-stage2", "educational_instruct", split="train", streaming=True)
    
    count = 0
    for item in iter(dataset):
        count += 1
        if count <= offset:
            continue
        
        try:
            instruction = item['instruction']
            output = item['output']
            combined_text = f"### Prompt:\n{instruction}\n### Response:\n{output}{END_TOKEN}"
            
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"opc_sft/file_counter": result.get('file_counter', 0),
                        f"opc_sft/processed_tokens": result.get('processed_tokens', 0),
                        f"opc_sft/requests_per_sec": result.get('requests_per_sec', 0),
                        f"opc_sft/samples": count,
                    })
            
            if count % SAVE_PROGRESS_INTERVAL == 0:
                update_progress('opc_sft', count)
                print(f"Progress saved: opc_sft at {count}")
        
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
    
    update_progress('opc_sft', count)
    print(f"Dataset opc_sft completed at {count}")

def process_codefeedback():
    SAVE_PERIOD_TO_RAG = 16
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get('codefeedback', 0)
    
    print(f"\n{'='*60}")
    print(f"Processing: OpenCoder-LLM/CodeFeedback-Filtered-Instruction")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    
    dataset = load_dataset("OpenCoder-LLM/CodeFeedback-Filtered-Instruction", split="train", streaming=True)
    
    count = 0
    for item in iter(dataset):
        count += 1
        if count <= offset:
            continue
        
        try:
            instruction = item['query']
            output = item['answer']
            lang = item.get('lang', '')
            combined_text = f"### Prompt:\n{instruction}\n### Language: {lang}\n### Response:\n{output}{END_TOKEN}"
            
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"codefeedback/file_counter": result.get('file_counter', 0),
                        f"codefeedback/processed_tokens": result.get('processed_tokens', 0),
                        f"codefeedback/requests_per_sec": result.get('requests_per_sec', 0),
                        f"codefeedback/samples": count,
                    })
            
            if count % SAVE_PROGRESS_INTERVAL == 0:
                update_progress('codefeedback', count)
                print(f"Progress saved: codefeedback at {count}")
        
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
    
    update_progress('codefeedback', count)
    print(f"Dataset codefeedback completed at {count}")

def process_leetcode():
    SAVE_PERIOD_TO_RAG = 16
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get('leetcode', 0)
    
    print(f"\n{'='*60}")
    print(f"Processing: newfacade/LeetCodeDataset")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    
    dataset = load_dataset("newfacade/LeetCodeDataset", split="train", streaming=True)
    
    count = 0
    for item in iter(dataset):
        count += 1
        if count <= offset:
            continue
        
        try:
            query = item['query']
            response = item['response']
            combined_text = f"### Prompt:\n{query}\n### Response:\n{response}{END_TOKEN}"
            
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"leetcode/file_counter": result.get('file_counter', 0),
                        f"leetcode/processed_tokens": result.get('processed_tokens', 0),
                        f"leetcode/requests_per_sec": result.get('requests_per_sec', 0),
                        f"leetcode/samples": count,
                    })
            
            if count % SAVE_PROGRESS_INTERVAL == 0:
                update_progress('leetcode', count)
                print(f"Progress saved: leetcode at {count}")
        
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
    
    update_progress('leetcode', count)
    print(f"Dataset leetcode completed at {count}")

def process_math_datasets():
    datasets_config = [
        ['nvidia/OpenMathReasoning', None, None, MATH_TEXTS, 'openmathre asoning'],
        ['OpenDataArena/ODA-Math-460k', 'train', None, MATH_TEXTS, 'oda_math'],
    ]
    
    SAVE_PERIOD_TO_RAG = 24
    SAVE_PROGRESS_INTERVAL = 512
    
    progress_data = load_progress()
    
    for config in datasets_config:
        dataset_name, split, subset, category, progress_key = config
        offset = progress_data.get(progress_key, 0)
        
        print(f"\n{'='*60}")
        print(f"Processing: {dataset_name}")
        print(f"Starting from offset: {offset}")
        print(f"{'='*60}\n")
        
        try:
            if split:
                dataset = load_dataset(dataset_name, split=split, streaming=True)
            else:
                ds = load_dataset(dataset_name)
                dataset = ds[list(ds.keys())[0]]
            
            count = 0
            for item in (iter(dataset) if hasattr(dataset, '__iter__') else dataset):
                count += 1
                if count <= offset:
                    continue
                
                try:
                    if 'openmathre' in progress_key:
                        problem = item['problem']
                        solution = item['generated_solution']
                        combined_text = f"### Problem:\n{problem}\n### Solution:\n{solution}{END_TOKEN}"
                    
                    elif 'oda_math' in progress_key:
                        question = item['question']
                        response = item['response']
                        expected = item.get('expected_answer', '')
                        combined_text = f"### Math Problem:\n{question}\n### Solution:\n{response}\n### Expected Answer:\n{expected}{END_TOKEN}"
                    
                    else:
                        continue
                    
                    if count % SAVE_PERIOD_TO_RAG == 0:
                        combined_text_rag = remove_think_tags(combined_text)
                        result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
                    
                    data = {
                        'text': combined_text,
                        'tokenized_text': tokenizer.encode(combined_text).ids,
                        'category': category
                    }
                    url = "http://localhost:4567/save-data"
                    response = session.post(url, json=data, timeout=5)
                    if response.status_code != 200:
                        print(f"Failed to save data: {response.text}")
                    else:
                        result = response.json()
                        if count % 500 == 0:
                            pprint(result)
                            if 'processed_tokens' in result:
                                print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                            wandb.log({
                                f"{progress_key}/file_counter": result.get('file_counter', 0),
                                f"{progress_key}/processed_tokens": result.get('processed_tokens', 0),
                                f"{progress_key}/requests_per_sec": result.get('requests_per_sec', 0),
                                f"{progress_key}/samples": count,
                            })
                    
                    if count % SAVE_PROGRESS_INTERVAL == 0:
                        update_progress(progress_key, count)
                        print(f"Progress saved: {progress_key} at {count}")
                
                except Exception as e:
                    print(f"Error processing item {count}: {e}")
                    continue
            
            update_progress(progress_key, count)
            print(f"Dataset {progress_key} completed at {count}")
        
        except Exception as e:
            print(f"Error loading dataset {dataset_name}: {e}")

def process_general_datasets():
    datasets_config = [
        ['open-thoughts/OpenThoughts3-1.2M', 'train', None, OPEN_THOUGHTS_TEXTS, 'openthoughts'],
        ['ccdv/arxiv-summarization', 'train', 'section', SCIENCE_TEXTS, 'arxiv'],
        ['m-a-p/FineFineWeb-sample', 'train', None, FINE_BERT_TEXTS, 'finefineweb'],
    ]
    
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 512
    
    progress_data = load_progress()
    
    for config in datasets_config:
        dataset_name, split, subset, category, progress_key = config
        offset = progress_data.get(progress_key, 0)
        
        print(f"\n{'='*60}")
        print(f"Processing: {dataset_name} ({subset if subset else 'default'})")
        print(f"Starting from offset: {offset}")
        print(f"{'='*60}\n")
        
        try:
            if subset:
                dataset = load_dataset(dataset_name, subset, split=split, streaming=True)
            else:
                dataset = load_dataset(dataset_name, split=split, streaming=True)
            
            count = 0
            for item in iter(dataset):
                count += 1
                if count <= offset:
                    continue
                
                try:
                    if 'openthoughts' in progress_key:
                        conversations = item['conversations']
                        combined_text = ""
                        for turn in conversations:
                            role = turn['from'].capitalize()
                            if role == 'Gpt':
                                role = 'Assistant'
                            combined_text += f"### {role}:\n{turn['value']}\n"
                        combined_text += END_TOKEN
                    
                    elif 'arxiv' in progress_key:
                        article = item['article']
                        abstract = item['abstract']
                        if len(article) < 50 or len(abstract) < 20:
                            continue
                        combined_text = f"### Article:\n{article}\n### Summary:\n{abstract}{END_TOKEN}"
                    
                    elif 'finefineweb' in progress_key:
                        text = item['text']
                        combined_text = f"{text}{END_TOKEN}"
                    
                    else:
                        continue
                    
                    if count % SAVE_PERIOD_TO_RAG == 0:
                        combined_text_rag = remove_think_tags(combined_text)
                        result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
                    
                    data = {
                        'text': combined_text,
                        'tokenized_text': tokenizer.encode(combined_text).ids,
                        'category': category
                    }
                    url = "http://localhost:4567/save-data"
                    response = session.post(url, json=data, timeout=5)
                    if response.status_code != 200:
                        print(f"Failed to save data: {response.text}")
                    else:
                        result = response.json()
                        if count % 500 == 0:
                            pprint(result)
                            if 'processed_tokens' in result:
                                print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                            wandb.log({
                                f"{progress_key}/file_counter": result.get('file_counter', 0),
                                f"{progress_key}/processed_tokens": result.get('processed_tokens', 0),
                                f"{progress_key}/requests_per_sec": result.get('requests_per_sec', 0),
                                f"{progress_key}/samples": count,
                            })
                    
                    if count % SAVE_PROGRESS_INTERVAL == 0:
                        update_progress(progress_key, count)
                        print(f"Progress saved: {progress_key} at {count}")
                
                except Exception as e:
                    print(f"Error processing item {count}: {e}")
                    continue
            
            update_progress(progress_key, count)
            print(f"Dataset {progress_key} completed at {count}")
        
        except Exception as e:
            print(f"Error loading dataset {dataset_name}: {e}")

def process_the_stack_language(lang, category, progress_key):
    SAVE_PERIOD_TO_RAG = 16
    SAVE_PROGRESS_INTERVAL = 512
    
    progress_data = load_progress()
    offset = progress_data.get(progress_key, 0)
    
    print(f"\n{'='*60}")
    print(f"Processing: the-stack ({lang})")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    
    try:
        dataset = load_dataset("bigcode/the-stack", streaming=True, split="train", data_dir=f"data/{lang}")
        
        count = 0
        for item in iter(dataset):
            count += 1
            if count <= offset:
                continue
            
            try:
                code = item['content']
                combined_text = f"{code}{END_TOKEN}"
                
                if count % SAVE_PERIOD_TO_RAG == 0:
                    result = embed_text_to_rag(combined_text, chunk_size=1024, overlap=128)
                
                data = {
                    'text': combined_text,
                    'tokenized_text': tokenizer.encode(combined_text).ids,
                    'category': category
                }
                url = "http://localhost:4567/save-data"
                response = session.post(url, json=data, timeout=5)
                if response.status_code != 200:
                    print(f"Failed to save data: {response.text}")
                else:
                    result = response.json()
                    if count % 500 == 0:
                        pprint(result)
                        if 'processed_tokens' in result:
                            print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                        wandb.log({
                            f"{progress_key}/file_counter": result.get('file_counter', 0),
                            f"{progress_key}/processed_tokens": result.get('processed_tokens', 0),
                            f"{progress_key}/requests_per_sec": result.get('requests_per_sec', 0),
                            f"{progress_key}/samples": count,
                        })
                
                if count % SAVE_PROGRESS_INTERVAL == 0:
                    update_progress(progress_key, count)
                    print(f"Progress saved: {progress_key} at {count}")
            
            except Exception as e:
                print(f"Error processing item {count}: {e}")
                continue
        
        update_progress(progress_key, count)
        print(f"Dataset {progress_key} completed at {count}")
    
    except Exception as e:
        print(f"Error loading the-stack ({lang}): {e}")

def process_stack_python():
    process_the_stack_language('python', CODE_MIX_TEXTS, 'stack_python')

def process_stack_go():
    process_the_stack_language('go', CODE_MIX_TEXTS, 'stack_go')

def process_stack_c():
    process_the_stack_language('c', CODE_MIX_TEXTS, 'stack_c')

def process_stack_cpp():
    process_the_stack_language('c++', CODE_MIX_TEXTS, 'stack_cpp')

def process_stack_rust():
    process_the_stack_language('rust', CODE_MIX_TEXTS, 'stack_rust')

def process_stack_javascript():
    process_the_stack_language('javascript', CODE_MIX_TEXTS, 'stack_js')

def process_stack_typescript():
    process_the_stack_language('typescript', CODE_MIX_TEXTS, 'stack_ts')

def process_stack_sql():
    process_the_stack_language('sql', CODE_MIX_TEXTS, 'stack_sql')

def process_stack_assembly():
    process_the_stack_language('assembly', CODE_MIX_TEXTS, 'stack_asm')

def process_the_stack_datasets_parallel():
    languages = [
        process_stack_python,
        process_stack_go,
        process_stack_c,
        process_stack_cpp,
        process_stack_rust,
        process_stack_javascript,
        process_stack_typescript,
        process_stack_sql,
        process_stack_assembly,
    ]
    
    with ProcessPoolExecutor(max_workers=min(len(languages), os.cpu_count())) as executor:
        futures = [executor.submit(func) for func in languages]
        for future in as_completed(futures):
            try:
                future.result()
            except Exception as e:
                print(f"Error in parallel processing: {e}")

def process_the_stack_datasets():
    languages = [
        ['python', CODE_MIX_TEXTS, 'stack_python'],
        ['go', CODE_MIX_TEXTS, 'stack_go'],
        ['c', CODE_MIX_TEXTS, 'stack_c'],
        ['c++', CODE_MIX_TEXTS, 'stack_cpp'],
        ['rust', CODE_MIX_TEXTS, 'stack_rust'],
        ['javascript', CODE_MIX_TEXTS, 'stack_js'],
        ['typescript', CODE_MIX_TEXTS, 'stack_ts'],
        ['sql', CODE_MIX_TEXTS, 'stack_sql'],
        ['assembly', CODE_MIX_TEXTS, 'stack_asm'],
    ]
    
    for lang_config in languages:
        lang, category, progress_key = lang_config
        process_the_stack_language(lang, category, progress_key)

def process_synthetic_1():
    ds = load_dataset("PrimeIntellect/SYNTHETIC-1", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get('synthetic_1', 0)
    print(f"\n{'='*60}")
    print(f"Processing: SYNTHETIC-1")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            prompt = item['prompt']
            llm_response = item['llm_response']
            gold_standard_solution = item['gold_standard_solution']
            combined_text = f"### Prompt:\n{prompt}\n### Response:\n{llm_response}\n### Gold Standard Solution:\n{str(gold_standard_solution)}\n"
            combined_text += END_TOKEN
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"synthetic_1/file_counter": result.get('file_counter', 0),
                        f"synthetic_1/processed_tokens": result.get('processed_tokens', 0),
                        f"synthetic_1/requests_per_sec": result.get('requests_per_sec', 0),
                        f"synthetic_1/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('synthetic_1', count)
            print(f"Progress saved: synthetic_1 at {count}")

    update_progress('synthetic_1', count)
    print(f"Dataset synthetic_1 completed at {count}")

def process_s1k():
    ds = load_dataset("simplescaling/s1K", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get('s1k', 0)
    print(f"\n{'='*60}")
    print(f"Processing: s1K")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            question = item['question']
            solution = item['solution']
            combined_text = f"### Question:\n{question}\n### Solution:\n{solution}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': MATH_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"s1k/file_counter": result.get('file_counter', 0),
                        f"s1k/processed_tokens": result.get('processed_tokens', 0),
                        f"s1k/requests_per_sec": result.get('requests_per_sec', 0),
                        f"s1k/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('s1k', count)
            print(f"Progress saved: s1k at {count}")    
    update_progress('s1k', count)
    print(f"Dataset s1k completed at {count}")

def process_natural_reasoning():
    ds = load_dataset("facebook/natural_reasoning", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get('natural_reasoning', 0)
    print(f"\n{'='*60}")
    print(f"Processing: natural_reasoning")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            question = item['question']
            dataset_response = item['responses'][0]['response']
            reference_answer = item['reference_answer']
            combined_text = f"### Question:\n{question}\n### Response:\n{dataset_response}\n### Reference Answer:\n{reference_answer}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"natural_reasoning/file_counter": result.get('file_counter', 0),
                        f"natural_reasoning/processed_tokens": result.get('processed_tokens', 0),
                        f"natural_reasoning/requests_per_sec": result.get('requests_per_sec', 0),
                        f"natural_reasoning/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('natural_reasoning', count)
            print(f"Progress saved: natural_reasoning at {count}")    
    update_progress('natural_reasoning', count)
    print(f"Dataset natural_reasoning completed at {count}")

def process_long():
    subjects = [
        ['advanced_math', MATH_TEXTS],
        ['advanced_physics', SCIENCE_TEXTS],
        ['chemistry', SCIENCE_TEXTS],
        ['computational_biology', SCIENCE_TEXTS],
        ['finance', GENERIC_TEXTS],
        ['games', GENERIC_TEXTS],
        ['graph_discrete_math', MATH_TEXTS],
        ['logic', MATH_TEXTS],
        ['mathematical_programming', MATH_TEXTS],
        ['medicine', SCIENCE_TEXTS],
        ['programming', CODE_MIX_TEXTS],
        ['security_and_safety', GENERIC_TEXTS]
    ]
    splits = ['train', 'test']
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    for subject_config in subjects:
        subject, category = subject_config
        for split in splits:
            progress_key = f"long_{subject}_{split}"
            offset = progress_data.get(progress_key, 0)
            print(f"\n{'='*60}")
            print(f"Processing: long ({subject}) - {split}")
            print(f"Starting from offset: {offset}")
            print(f"{'='*60}\n")
            try:
                dataset = load_dataset("camel-ai/loong", subject, split=split, streaming=True)
                count = 0
                for item in iter(dataset):
                    count += 1
                    if count <= offset:
                        continue
                    try:
                        question = item['question']
                        rationale = item['rationale']
                        final_answer = item['final_answer']
                        combined_text = f"### Question:\n{question}\n### Rationale:\n{rationale}\n### Final Answer:\n{final_answer}{END_TOKEN}"
                        if count % SAVE_PERIOD_TO_RAG == 0:
                            combined_text_rag = remove_think_tags(combined_text)
                            result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
                        data = {
                            'text': combined_text,
                            'tokenized_text': tokenizer.encode(combined_text).ids,
                            'category': category
                        }
                        url = "http://localhost:4567/save-data"
                        response = session.post(url, json=data, timeout=5)
                        if response.status_code != 200:
                            print(f"Failed to save data: {response.text}")
                        else:
                            result = response.json()
                            if count % 500 == 0:
                                pprint(result)
                                if 'processed_tokens' in result:
                                    print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                                wandb.log({
                                    f"{progress_key}/file_counter": result.get('file_counter', 0),
                                    f"{progress_key}/processed_tokens": result.get('processed_tokens', 0),
                                    f"{progress_key}/requests_per_sec": result.get('requests_per_sec', 0),
                                    f"{progress_key}/samples": count,
                                })
                        if count % SAVE_PROGRESS_INTERVAL == 0:
                            update_progress(progress_key, count)
                            print(f"Progress saved: {progress_key} at {count}")
                    except Exception as e:
                        print(f"Error processing item {count}: {e}")
                        continue
                update_progress(progress_key, count)
                print(f"Dataset {progress_key} completed at {count}")
            except Exception as e:
                print(f"Error loading long ({subject}) - {split}: {e}")
        
def process_stackoverflow():
    ds = load_dataset("suriyagunasekar/stackoverflow-with-meta-data", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 512
    progress_data = load_progress()
    offset = progress_data.get('stackoverflow', 0)
    print(f"\n{'='*60}")
    print(f"Processing: stackoverflow")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            content = item['content']
            combined_text = f"{content}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")  
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"stackoverflow/file_counter": result.get('file_counter', 0),
                        f"stackoverflow/processed_tokens": result.get('processed_tokens', 0),
                        f"stackoverflow/requests_per_sec": result.get('requests_per_sec', 0),
                        f"stackoverflow/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('stackoverflow', count)
            print(f"Progress saved: stackoverflow at {count}")    
    update_progress('stackoverflow', count)
    print(f"Dataset stackoverflow completed at {count}")

def process_truthful_qa():
    ds = load_dataset("truthfulqa/truthful_qa", "generation", split="validation", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('truthful_qa', 0)
    print(f"\n{'='*60}")
    print(f"Processing: truthful_qa")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            qestion = item['question']
            best_answer = item['best_answer']
            combined_text = f"### Question:\n{qestion}\n### Response:\n{best_answer}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")  
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"truthful_qa/file_counter": result.get('file_counter', 0),
                        f"truthful_qa/processed_tokens": result.get('processed_tokens', 0),
                        f"truthful_qa/requests_per_sec": result.get('requests_per_sec', 0),
                        f"truthful_qa/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('truthful_qa', count)
            print(f"Progress saved: truthful_qa at {count}")    
    update_progress('truthful_qa', count)
    print(f"Dataset truthful_qa completed at {count}")

def process_linux_commands():
    dataset = load_dataset("missvector/linux-commands", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('linux_commands', 0)
    print(f"\n{'='*60}")
    print(f"Processing: linux_commands")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(dataset):
        count += 1
        if count <= offset:
            continue
        try:
            prompt = item['eng']
            command = item['completion']
            combined_text = f"### Convert to Linux command:\n{prompt}\n### Command:\n{command}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"linux_commands/file_counter": result.get('file_counter', 0),
                        f"linux_commands/processed_tokens": result.get('processed_tokens', 0),
                        f"linux_commands/requests_per_sec": result.get('requests_per_sec', 0),
                        f"linux_commands/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('linux_commands', count)
            print(f"Progress saved: linux_commands at {count}")
    update_progress('linux_commands', count)
    print(f"Dataset linux_commands completed at {count}")

def process_mmlu():
    subjects = ['abstract_algebra', 'anatomy', 'astronomy', 'business_ethics', 'clinical_knowledge', 'college_biology', 'college_chemistry', 'college_computer_science', 'college_mathematics', 'college_medicine', 'college_physics', 'computer_security', 'conceptual_physics', 'econometrics', 'electrical_engineering', 'elementary_mathematics', 'formal_logic', 'global_facts', 'high_school_biology', 'high_school_chemistry', 'high_school_computer_science', 'high_school_european_history', 'high_school_geography', 'high_school_government_and_politics', 'high_school_macroeconomics', 'high_school_mathematics', 'high_school_microeconomics', 'high_school_physics', 'high_school_psychology', 'high_school_statistics', 'high_school_us_history', 'high_school_world_history', 'human_aging', 'human_sexuality', 'international_law', 'jurisprudence', 'logical_fallacies', 'machine_learning', 'management', 'marketing', 'medical_genetics', 'miscellaneous', 'moral_disputes', 'moral_scenarios', 'nutrition', 'philosophy', 'prehistory', 'professional_accounting', 'professional_law', 'professional_medicine', 'professional_psychology', 'public_relations', 'security_studies', 'sociology', 'us_foreign_policy', 'virology', 'world_religions']
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 128
    progress_data = load_progress()
    for subject in subjects:
        progress_key = f"mmlu_{subject}"
        offset = progress_data.get(progress_key, 0)
        print(f"\n{'='*60}")
        print(f"Processing: MMLU ({subject})")
        print(f"Starting from offset: {offset}")
        print(f"{'='*60}\n")
        try:
            ds = load_dataset("cais/mmlu", subject, split="test", streaming=True)
            count = 0
            for item in iter(ds):
                count += 1
                if count <= offset:
                    continue
                try:
                    question = item['question']
                    choices = item['choices']
                    answer_idx = item['answer']
                    combined_text = f"### Question:\n{question}\n### Choices:\n"
                    for i, choice in enumerate(choices):
                        combined_text += f"{chr(65+i)}. {choice}\n"
                    combined_text += f"### Answer:\n{chr(65+answer_idx)}{END_TOKEN}"
                    if count % SAVE_PERIOD_TO_RAG == 0:
                        combined_text_rag = remove_think_tags(combined_text)
                        result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
                    data = {
                        'text': combined_text,
                        'tokenized_text': tokenizer.encode(combined_text).ids,
                        'category': GENERIC_TEXTS
                    }
                    url = "http://localhost:4567/save-data"
                    response = session.post(url, json=data, timeout=5)
                    if response.status_code != 200:
                        print(f"Failed to save data: {response.text}")
                    else:
                        result = response.json()
                        if count % 500 == 0:
                            pprint(result)
                            if 'processed_tokens' in result:
                                print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                            wandb.log({
                                f"{progress_key}/file_counter": result.get('file_counter', 0),
                                f"{progress_key}/processed_tokens": result.get('processed_tokens', 0),
                                f"{progress_key}/requests_per_sec": result.get('requests_per_sec', 0),
                                f"{progress_key}/samples": count,
                            })
                    if count % SAVE_PROGRESS_INTERVAL == 0:
                        update_progress(progress_key, count)
                        print(f"Progress saved: {progress_key} at {count}")
                except Exception as e:
                    print(f"Error processing item {count}: {e}")
                    continue
            update_progress(progress_key, count)
            print(f"Dataset {progress_key} completed at {count}")
        except Exception as e:
            print(f"Error loading MMLU ({subject}): {e}")

def process_cosmo_1B_claude_taxonomy220():
    ds = load_dataset("LGizkde/cosmo-1B-claude_taxonomy220", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('cosmo_1B_claude_taxonomy220', 0)
    print(f"\n{'='*60}")
    print(f"Processing: cosmo_1B_claude_taxonomy220")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            prompt = item['prompt']
            response = item['text']
            combined_text = f"### Prompt:\n{prompt}\n### Response:\n{response}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"cosmo_1B_claude_taxonomy220/file_counter": result.get('file_counter', 0),
                        f"cosmo_1B_claude_taxonomy220/processed_tokens": result.get('processed_tokens', 0),
                        f"cosmo_1B_claude_taxonomy220/requests_per_sec": result.get('requests_per_sec', 0),
                        f"cosmo_1B_claude_taxonomy220/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('cosmo_1B_claude_taxonomy220', count)
            print(f"Progress saved: cosmo_1B_claude_taxonomy220 at {count}")    
    update_progress('cosmo_1B_claude_taxonomy220', count)
    print(f"Dataset cosmo_1B_claude_taxonomy220 completed at {count}")

def process_oh_dcft_v3_1_claude_3_5_haiku_20241022():
    ds = load_dataset("mlfoundations-dev/oh-dcft-v3.1-claude-3-5-haiku-20241022", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('oh_dcft_v3_1_claude_3_5_haiku_20241022', 0)
    print(f"\n{'='*60}")
    print(f"Processing: oh_dcft_v3_1_claude_3_5_haiku_20241022")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            conversation = item['conversations']
            combined_text = ""
            for turn in conversation:
                if turn['from'] == 'human':
                    combined_text += f"### Prompt:\n{turn['value']}\n"
                elif turn['from'] == 'gpt':
                    combined_text += f"### Response:\n{turn['value']}\n"
            combined_text += END_TOKEN
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"oh_dcft_v3_1_claude_3_5_haiku_20241022/file_counter": result.get('file_counter', 0),
                        f"oh_dcft_v3_1_claude_3_5_haiku_20241022/processed_tokens": result.get('processed_tokens', 0),
                        f"oh_dcft_v3_1_claude_3_5_haiku_20241022/requests_per_sec": result.get('requests_per_sec', 0),
                        f"oh_dcft_v3_1_claude_3_5_haiku_20241022/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('oh_dcft_v3_1_claude_3_5_haiku_20241022', count)
            print(f"Progress saved: oh_dcft_v3_1_claude_3_5_haiku_20241022 at {count}")    
    update_progress('oh_dcft_v3_1_claude_3_5_haiku_20241022', count)
    print(f"Dataset oh_dcft_v3_1_claude_3_5_haiku_20241022 completed at {count}")

def proccess_chatbot_arena_conversations():
    ds = load_dataset("lmsys/chatbot_arena_conversations", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 128
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('chatbot_arena_conversations', 0)
    print(f"\n{'='*60}")
    print(f"Processing: chatbot_arena_conversations")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            conversation_b = item['conversation_b']
            conversation_a = item['conversation_a']
            conversation_b_text = ""
            for turn in conversation_b:
                if turn['role'] == 'user':
                    conversation_b_text += f"### Prompt:\n{turn['content']}\n"
                elif turn['role'] == 'assistant':
                    conversation_b_text += f"### Response:\n{turn['content']}\n"
            conversation_a_text = ""
            for turn in conversation_a:
                if turn['role'] == 'user':
                    conversation_a_text += f"### Prompt:\n{turn['content']}\n"
                elif turn['role'] == 'assistant':
                    conversation_a_text += f"### Response:\n{turn['content']}\n"
            combined_text = f"### Conversation A:\n{conversation_a_text}\n### Conversation B:\n{conversation_b_text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"chatbot_arena_conversations/file_counter": result.get('file_counter', 0),
                        f"chatbot_arena_conversations/processed_tokens": result.get('processed_tokens', 0),
                        f"chatbot_arena_conversations/requests_per_sec": result.get('requests_per_sec', 0),
                        f"chatbot_arena_conversations/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('chatbot_arena_conversations', count)
            print(f"Progress saved: chatbot_arena_conversations at {count}")    
    update_progress('chatbot_arena_conversations', count)
    print(f"Dataset chatbot_arena_conversations completed at {count}")

def process_reasoning_conversations_advanced_1m():
    ds = load_dataset("naimulislam/reasoning_conversations_advanced_1m", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 128
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('reasoning_conversations_advanced_1m', 0)
    print(f"\n{'='*60}")
    print(f"Processing: reasoning_conversations_advanced_1m")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            question = item['question']
            final_answer = item['final_answer']
            combined_text = f"### Question:\n{question}\n### Final Answer:\n{final_answer}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"reasoning_conversations_advanced_1m/file_counter": result.get('file_counter', 0),
                        f"reasoning_conversations_advanced_1m/processed_tokens": result.get('processed_tokens', 0),
                        f"reasoning_conversations_advanced_1m/requests_per_sec": result.get('requests_per_sec', 0),
                        f"reasoning_conversations_advanced_1m/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('reasoning_conversations_advanced_1m', count)
            print(f"Progress saved: reasoning_conversations_advanced_1m at {count}")    
    update_progress('reasoning_conversations_advanced_1m', count)
    print(f"Dataset reasoning_conversations_advanced_1m completed at {count}")


def process_reason_code_search_net_python():
    ds = load_dataset("Nan-Do/reason_code-search-net-python", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('reason_code_search_net_python', 0)
    print(f"\n{'='*60}")
    print(f"Processing: reason_code_search_net_python")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            prompt = item['INSTRUCTION']
            response = item['RESPONSE']
            combined_text = f"### Instruction:\n{prompt}\n### Response:\n{response}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"reason_code_search_net_python/file_counter": result.get('file_counter', 0),
                        f"reason_code_search_net_python/processed_tokens": result.get('processed_tokens', 0),
                        f"reason_code_search_net_python/requests_per_sec": result.get('requests_per_sec', 0),
                        f"reason_code_search_net_python/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('reason_code_search_net_python', count)
            print(f"Progress saved: reason_code_search_net_python at {count}")    
    update_progress('reason_code_search_net_python', count)
    print(f"Dataset reason_code_search_net_python completed at {count}")

def process_wiki_qa():
    ds = load_dataset("microsoft/wiki_qa", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 24
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('wiki_qa', 0)
    print(f"\n{'='*60}")
    print(f"Processing: wiki_qa")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            qestion = item['question']
            answer = item['answer']
            combined_text = f"### Prompt:\n{qestion}\n### Response:\n{answer}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"wiki_qa/file_counter": result.get('file_counter', 0),
                        f"wiki_qa/processed_tokens": result.get('processed_tokens', 0),
                        f"wiki_qa/requests_per_sec": result.get('requests_per_sec', 0),
                        f"wiki_qa/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('wiki_qa', count)
            print(f"Progress saved: wiki_qa at {count}")
    update_progress('wiki_qa', count)
    print(f"Dataset wiki_qa completed at {count}")

def process_triviaqa():
    ds = load_dataset("lucadiliello/triviaqa", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 24
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('triviaqa', 0)
    print(f"\n{'='*60}")
    print(f"Processing: triviaqa")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            qestion = item['question']
            answer = item['answers'][0]
            combined_text = f"### Prompt:\n{qestion}\n### Response:\n{answer}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"triviaqa/file_counter": result.get('file_counter', 0),
                        f"triviaqa/processed_tokens": result.get('processed_tokens', 0),
                        f"triviaqa/requests_per_sec": result.get('requests_per_sec', 0),
                        f"triviaqa/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('triviaqa', count)
            print(f"Progress saved: triviaqa at {count}")    
    update_progress('triviaqa', count)
    print(f"Dataset triviaqa completed at {count}")
            
def process_dolphin_r1():
    subsets = ['nonreasoning', 'reasoning-deepseek', 'reasoning-flash']
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    for subset in subsets:
        progress_key = f"dolphin_r1_{subset}"
        offset = progress_data.get(progress_key, 0)
        print(f"\n{'='*60}")
        print(f"Processing: dolphin_r1 ({subset})")
        print(f"Starting from offset: {offset}")
        print(f"{'='*60}\n")
        try:
            ds = load_dataset("QuixiAI/dolphin-r1", subset, split="train", streaming=True)
            count = 0
            for item in iter(ds):
                count += 1
                if count <= offset:
                    continue
                try:
                    if subset == 'nonreasoning':
                        messages = item['messages']
                        combined_text = ""
                        for turn in messages:
                            if turn['role'] == 'user':
                                combined_text += f"### Prompt:\n{turn['content']}\n"
                            elif turn['role'] == 'assistant':
                                combined_text += f"### Response:\n{turn['content']}\n"
                        combined_text += END_TOKEN
                    else:
                        messages = item['messages']
                        combined_text = ""
                        for turn in messages:
                            if turn['role'] == 'user':
                                combined_text += f"### Prompt:\n{turn['content']}\n"
                        reasoning = item['reasoning']
                        answer = item['answer']
                        combined_text += f"### Reasoning:\n{reasoning}\n### Answer:\n{answer}{END_TOKEN}"
                    if count % SAVE_PERIOD_TO_RAG == 0:
                        combined_text_rag = remove_think_tags(combined_text)
                        result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
                    data = {
                        'text': combined_text,
                        'tokenized_text': tokenizer.encode(combined_text).ids,
                        'category': GENERIC_TEXTS
                    }
                    url = "http://localhost:4567/save-data"
                    response = session.post(url, json=data, timeout=5)
                    if response.status_code != 200:
                        print(f"Failed to save data: {response.text}")
                    else:
                        result = response.json()
                        if count % 500 == 0:
                            pprint(result)
                            if 'processed_tokens' in result:
                                print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                            wandb.log({
                                f"{progress_key}/file_counter": result.get('file_counter', 0),
                                f"{progress_key}/processed_tokens": result.get('processed_tokens', 0),
                                f"{progress_key}/requests_per_sec": result.get('requests_per_sec', 0),
                                f"{progress_key}/samples": count,
                            })
                    if count % SAVE_PROGRESS_INTERVAL == 0:
                        update_progress(progress_key, count)
                        print(f"Progress saved: {progress_key} at {count}")
                except Exception as e:
                    print(f"Error processing item {count}: {e}")
                    continue
            update_progress(progress_key, count)
            print(f"Dataset {progress_key} completed at {count}")
        except Exception as e:
            print(f"Error loading dolphin_r1 ({subset}): {e}")

def process_textbooks_are_all_you_need_lite():
    ds = load_dataset("SciPhi/textbooks-are-all-you-need-lite", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('textbooks_are_all_you_need_lite', 0)
    print(f"\n{'='*60}")
    print(f"Processing: textbooks_are_all_you_need_lite")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            prompt = item['formatted_prompt']
            response = item['completion']
            combined_text = f"### Prompt:\n{prompt}\n### Response:\n{response}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"textbooks_are_all_you_need_lite/file_counter": result.get('file_counter', 0),
                        f"textbooks_are_all_you_need_lite/processed_tokens": result.get('processed_tokens', 0),
                        f"textbooks_are_all_you_need_lite/requests_per_sec": result.get('requests_per_sec', 0),
                        f"textbooks_are_all_you_need_lite/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('textbooks_are_all_you_need_lite', count)
            print(f"Progress saved: textbooks_are_all_you_need_lite at {count}")    
    update_progress('textbooks_are_all_you_need_lite', count)
    print(f"Dataset textbooks_are_all_you_need_lite completed at {count}")

def process_leet10k_alpaca():
    ds = load_dataset("QuixiAI/leet10k-alpaca", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('leet10k_alpaca', 0)
    print(f"\n{'='*60}")
    print(f"Processing: leet10k_alpaca")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            prompt = item['instruction']
            _input = item['input']
            response = item['output']

            combined_text = f"### Instruction:\n{prompt}\n### Input:\n{_input}\n### Response:\n{response}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"leet10k_alpaca/file_counter": result.get('file_counter', 0),
                        f"leet10k_alpaca/processed_tokens": result.get('processed_tokens', 0),
                        f"leet10k_alpaca/requests_per_sec": result.get('requests_per_sec', 0),
                        f"leet10k_alpaca/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('leet10k_alpaca', count)
            print(f"Progress saved: leet10k_alpaca at {count}")    
    update_progress('leet10k_alpaca', count)
    print(f"Dataset leet10k_alpaca completed at {count}")

def process_TextbookReasoning():
    ds = load_dataset("MegaScience/TextbookReasoning", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('TextbookReasoning', 0)
    print(f"\n{'='*60}")
    print(f"Processing: TextbookReasoning")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            prompt = item['question']
            response = item['answer']
            reference_answer = item['reference_answer']
            combined_text = f"### Question:\n{prompt}\n### Answer:\n{response}\n### Reference Answers:\n {reference_answer}\n" + END_TOKEN
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"TextbookReasoning/file_counter": result.get('file_counter', 0),
                        f"TextbookReasoning/processed_tokens": result.get('processed_tokens', 0),
                        f"TextbookReasoning/requests_per_sec": result.get('requests_per_sec', 0),
                        f"TextbookReasoning/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('TextbookReasoning', count)
            print(f"Progress saved: TextbookReasoning at {count}")    
    update_progress('TextbookReasoning', count)
    print(f"Dataset TextbookReasoning completed at {count}")

def process_cosmopedia_v2_textbook_and_howto_8_3m():
    ds = load_dataset("schuler/cosmopedia-v2-textbook-and-howto-8.3m", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('cosmopedia_v2_textbook_and_howto_8_3m', 0)
    print(f"\n{'='*60}")
    print(f"Processing: cosmopedia_v2_textbook_and_howto_8_3m")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"cosmopedia_v2_textbook_and_howto_8_3m/file_counter": result.get('file_counter', 0),
                        f"cosmopedia_v2_textbook_and_howto_8_3m/processed_tokens": result.get('processed_tokens', 0),
                        f"cosmopedia_v2_textbook_and_howto_8_3m/requests_per_sec": result.get('requests_per_sec', 0),
                        f"cosmopedia_v2_textbook_and_howto_8_3m/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('cosmopedia_v2_textbook_and_howto_8_3m', count)
            print(f"Progress saved: cosmopedia_v2_textbook_and_howto_8_3m at {count}")    
    update_progress('cosmopedia_v2_textbook_and_howto_8_3m', count)
    print(f"Dataset cosmopedia_v2_textbook_and_howto_8_3m completed at {count}")

def process_science_theory_textbooks():
    ds = load_dataset("Nbardy/science-theory-textbooks", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('science_theory_textbooks', 0)
    print(f"\n{'='*60}")
    print(f"Processing: science_theory_textbooks")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"science_theory_textbooks/file_counter": result.get('file_counter', 0),
                        f"science_theory_textbooks/processed_tokens": result.get('processed_tokens', 0),
                        f"science_theory_textbooks/requests_per_sec": result.get('requests_per_sec', 0),
                        f"science_theory_textbooks/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('science_theory_textbooks', count)
            print(f"Progress saved: science_theory_textbooks at {count}")    
    update_progress('science_theory_textbooks', count)
    print(f"Dataset science_theory_textbooks completed at {count}")

def proccess_the_pile_github():
    ds = load_dataset("andstor/the_pile_github", "all", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('the_pile_github', 0)
    print(f"\n{'='*60}")
    print(f"Processing: the_pile_github")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"the_pile_github/file_counter": result.get('file_counter', 0),
                        f"the_pile_github/processed_tokens": result.get('processed_tokens', 0),
                        f"the_pile_github/requests_per_sec": result.get('requests_per_sec', 0),
                        f"the_pile_github/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('the_pile_github', count)
            print(f"Progress saved: the_pile_github at {count}")    
    update_progress('the_pile_github', count)
    print(f"Dataset the_pile_github completed at {count}")

def procces_ODA_Mixture_100k():
    ds = load_dataset("OpenDataArena/ODA-Mixture-100k", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('ODA_Mixture_100k', 0)
    print(f"\n{'='*60}")
    print(f"Processing: ODA_Mixture_100k")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            prompt = item['instruction']
            response = item['response']
            combined_text = f"### Instruction:\n{prompt}\n### Response:\n{response}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"ODA_Mixture_100k/file_counter": result.get('file_counter', 0),
                        f"ODA_Mixture_100k/processed_tokens": result.get('processed_tokens', 0),
                        f"ODA_Mixture_100k/requests_per_sec": result.get('requests_per_sec', 0),
                        f"ODA_Mixture_100k/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('ODA_Mixture_100k', count)
            print(f"Progress saved: ODA_Mixture_100k at {count}")    
    update_progress('ODA_Mixture_100k', count)
    print(f"Dataset ODA_Mixture_100k completed at {count}")

def process_golang_coder():
    ds = load_dataset("smcleod/golang-coder", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('golang_coder', 0)
    print(f"\n{'='*60}")
    print(f"Processing: golang_coder")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            messeges = item['messages']
            combined_text = ""
            for turn in messeges:
                if turn['role'] == 'user':
                    combined_text += f"### Prompt:\n{turn['content']}\n"
                elif turn['role'] == 'assistant':
                    combined_text += f"### Response:\n{turn['content']}\n"
            combined_text += END_TOKEN
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"golang_coder/file_counter": result.get('file_counter', 0),
                        f"golang_coder/processed_tokens": result.get('processed_tokens', 0),
                        f"golang_coder/requests_per_sec": result.get('requests_per_sec', 0),
                        f"golang_coder/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('golang_coder', count)
            print(f"Progress saved: golang_coder at {count}")    
    update_progress('golang_coder', count)
    print(f"Dataset golang_coder completed at {count}")

def process_LiteCoder_SFT_Terminal_preview():
    ds = load_dataset("Lite-Coder/LiteCoder-SFT-Terminal-preview", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('LiteCoder_SFT_Terminal_preview', 0)
    print(f"\n{'='*60}")
    print(f"Processing: LiteCoder_SFT_Terminal_preview")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            conversations = item['conversations']
            combined_text = ""
            for turn in conversations:
                if turn['from'] == 'human':
                    combined_text += f"### Prompt:\n{turn['value']}\n"
                elif turn['from'] == 'gpt':
                    combined_text += f"### Response:\n{turn['value']}\n"
            combined_text += END_TOKEN
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"LiteCoder_SFT_Terminal_preview/file_counter": result.get('file_counter', 0),
                        f"LiteCoder_SFT_Terminal_preview/processed_tokens": result.get('processed_tokens', 0),
                        f"LiteCoder_SFT_Terminal_preview/requests_per_sec": result.get('requests_per_sec', 0),
                        f"LiteCoder_SFT_Terminal_preview/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('LiteCoder_SFT_Terminal_preview', count)
            print(f"Progress saved: LiteCoder_SFT_Terminal_preview at {count}")    
    update_progress('LiteCoder_SFT_Terminal_preview', count)
    print(f"Dataset LiteCoder_SFT_Terminal_preview completed at {count}")

def process_dolphin_coder():
    ds = load_dataset("QuixiAI/dolphin-coder", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('dolphin_coder', 0)
    print(f"\n{'='*60}")
    print(f"Processing: dolphin_coder")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            qestion = item['question']
            response = item['response']
            combined_text = f"### Prompt:\n{qestion}\n### Response:\n{response}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"dolphin_coder/file_counter": result.get('file_counter', 0),
                        f"dolphin_coder/processed_tokens": result.get('processed_tokens', 0),
                        f"dolphin_coder/requests_per_sec": result.get('requests_per_sec', 0),
                        f"dolphin_coder/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('dolphin_coder', count)
            print(f"Progress saved: dolphin_coder at {count}")    
    update_progress('dolphin_coder', count)
    print(f"Dataset dolphin_coder completed at {count}")

def proccess_textbook_codex():
    ds = load_dataset("crumb/textbook-codex", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('textbook_codex', 0)
    print(f"\n{'='*60}")
    print(f"Processing: textbook_codex")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"textbook_codex/file_counter": result.get('file_counter', 0),
                        f"textbook_codex/processed_tokens": result.get('processed_tokens', 0),
                        f"textbook_codex/requests_per_sec": result.get('requests_per_sec', 0),
                        f"textbook_codex/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('textbook_codex', count)
            print(f"Progress saved: textbook_codex at {count}")    
    update_progress('textbook_codex', count)
    print(f"Dataset textbook_codex completed at {count}")

def process_OpenCodeReasoning():
    ds = load_dataset("nvidia/OpenCodeReasoning", "split_0", split="split_0", streaming=True)
    SAVE_PERIOD_TO_RAG = 32
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('OpenCodeReasoning', 0)
    print(f"\n{'='*60}")
    print(f"Processing: OpenCodeReasoning")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            prompt = item['input']
            response = item['output']
            combined_text = f"### Prompt:\n{prompt}\n### Response:\n{response}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"OpenCodeReasoning/file_counter": result.get('file_counter', 0),
                        f"OpenCodeReasoning/processed_tokens": result.get('processed_tokens', 0),
                        f"OpenCodeReasoning/requests_per_sec": result.get('requests_per_sec', 0),
                        f"OpenCodeReasoning/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('OpenCodeReasoning', count)
            print(f"Progress saved: OpenCodeReasoning at {count}")    
    update_progress('OpenCodeReasoning', count)
    print(f"Dataset OpenCodeReasoning completed at {count}")

def process_pile():
    ds = load_dataset("monology/pile-uncopyrighted", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 128
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('pile', 0)
    print(f"\n{'='*60}")
    print(f"Processing: pile")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"pile/file_counter": result.get('file_counter', 0),
                        f"pile/processed_tokens": result.get('processed_tokens', 0),
                        f"pile/requests_per_sec": result.get('requests_per_sec', 0),
                        f"pile/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('pile', count)
            print(f"Progress saved: pile at {count}")    
    update_progress('pile', count)
    print(f"Dataset pile completed at {count}")

def proccess_MegaMath_Web_Pro_Max():
    ds = load_dataset("OctoThinker/MegaMath-Web-Pro-Max", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 256
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('MegaMath_Web_Pro_Max', 0)
    print(f"\n{'='*60}")
    print(f"Processing: MegaMath_Web_Pro_Max")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': MATH_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"MegaMath_Web_Pro_Max/file_counter": result.get('file_counter', 0),
                        f"MegaMath_Web_Pro_Max/processed_tokens": result.get('processed_tokens', 0),
                        f"MegaMath_Web_Pro_Max/requests_per_sec": result.get('requests_per_sec', 0),
                        f"MegaMath_Web_Pro_Max/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('MegaMath_Web_Pro_Max', count)
            print(f"Progress saved: MegaMath_Web_Pro_Max at {count}")    
    update_progress('MegaMath_Web_Pro_Max', count)
    print(f"Dataset MegaMath_Web_Pro_Max completed at {count}")

def proccess_FineFineWeb():
    ds = load_dataset("m-a-p/FineFineWeb", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 512
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('FineFineWeb', 0)
    print(f"\n{'='*60}")
    print(f"Processing: FineFineWeb")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"FineFineWeb/file_counter": result.get('file_counter', 0),
                        f"FineFineWeb/processed_tokens": result.get('processed_tokens', 0),
                        f"FineFineWeb/requests_per_sec": result.get('requests_per_sec', 0),
                        f"FineFineWeb/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('FineFineWeb', count)
            print(f"Progress saved: FineFineWeb at {count}")    
    update_progress('FineFineWeb', count)
    print(f"Dataset FineFineWeb completed at {count}")

def proccess_pretraining_v1_omega_books():
    ds = load_dataset("applied-ai-018/pretraining_v1-omega_books", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 512
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('pretraining_v1_omega_books', 0)
    print(f"\n{'='*60}")
    print(f"Processing: pretraining_v1_omega_books")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"pretraining_v1_omega_books/file_counter": result.get('file_counter', 0),
                        f"pretraining_v1_omega_books/processed_tokens": result.get('processed_tokens', 0),
                        f"pretraining_v1_omega_books/requests_per_sec": result.get('requests_per_sec', 0),
                        f"pretraining_v1_omega_books/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('pretraining_v1_omega_books', count)
            print(f"Progress saved: pretraining_v1_omega_books at {count}")    
    update_progress('pretraining_v1_omega_books', count)
    print(f"Dataset pretraining_v1_omega_books completed at {count}")

def proccess_c4():
    en = load_dataset("allenai/c4", "en", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 512
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('c4', 0)
    print(f"\n{'='*60}")
    print(f"Processing: c4")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(en):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"c4/file_counter": result.get('file_counter', 0),
                        f"c4/processed_tokens": result.get('processed_tokens', 0),
                        f"c4/requests_per_sec": result.get('requests_per_sec', 0),
                        f"c4/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('c4', count)
            print(f"Progress saved: c4 at {count}")    
    update_progress('c4', count)
    print(f"Dataset c4 completed at {count}")

def process_reddit_dataset_157():
    ds = load_dataset("tensorshield/reddit_dataset_157", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 512
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('reddit_dataset_157', 0)
    print(f"\n{'='*60}")
    print(f"Processing: reddit_dataset_157")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"reddit_dataset_157/file_counter": result.get('file_counter', 0),
                        f"reddit_dataset_157/processed_tokens": result.get('processed_tokens', 0),
                        f"reddit_dataset_157/requests_per_sec": result.get('requests_per_sec', 0),
                        f"reddit_dataset_157/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('reddit_dataset_157', count)
            print(f"Progress saved: reddit_dataset_157 at {count}")    
    update_progress('reddit_dataset_157', count)
    print(f"Dataset reddit_dataset_157 completed at {count}")

def procces_top_reddit_posts_daily():
    ds = load_dataset("hblim/top_reddit_posts_daily", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 512
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('top_reddit_posts_daily', 0)
    print(f"\n{'='*60}")
    print(f"Processing: top_reddit_posts_daily")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"top_reddit_posts_daily/file_counter": result.get('file_counter', 0),
                        f"top_reddit_posts_daily/processed_tokens": result.get('processed_tokens', 0),
                        f"top_reddit_posts_daily/requests_per_sec": result.get('requests_per_sec', 0),
                        f"top_reddit_posts_daily/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('top_reddit_posts_daily', count)
            print(f"Progress saved: top_reddit_posts_daily at {count}")    
    update_progress('top_reddit_posts_daily', count)
    print(f"Dataset top_reddit_posts_daily completed at {count}")

def procces_codesearchnet_qa():
    ds = load_dataset("aalexchengg/codesearchnet_qa", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 512
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('codesearchnet_qa', 0)
    print(f"\n{'='*60}")
    print(f"Processing: codesearchnet_qa")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            whole_func_string = item['whole_func_string']
            question = item['question']
            answer = item['answer']
            answer = " ".join(answer)
            combined_text = f"### Function:\n{whole_func_string}\n### Question:\n{question}\n### Answer:\n{answer}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"codesearchnet_qa/file_counter": result.get('file_counter', 0),
                        f"codesearchnet_qa/processed_tokens": result.get('processed_tokens', 0),
                        f"codesearchnet_qa/requests_per_sec": result.get('requests_per_sec', 0),
                        f"codesearchnet_qa/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('codesearchnet_qa', count)
            print(f"Progress saved: codesearchnet_qa at {count}")    
    update_progress('codesearchnet_qa', count)
    print(f"Dataset codesearchnet_qa completed at {count}")

def procces_comma_v0_1_training_dataset():
    ds = load_dataset("common-pile/comma_v0.1_training_dataset", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 512
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('comma_v0_1_training_dataset', 0)
    print(f"\n{'='*60}")
    print(f"Processing: comma_v0_1_training_dataset")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': CODE_MIX_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"comma_v0_1_training_dataset/file_counter": result.get('file_counter', 0),
                        f"comma_v0_1_training_dataset/processed_tokens": result.get('processed_tokens', 0),
                        f"comma_v0_1_training_dataset/requests_per_sec": result.get('requests_per_sec', 0),
                        f"comma_v0_1_training_dataset/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('comma_v0_1_training_dataset', count)
            print(f"Progress saved: comma_v0_1_training_dataset at {count}")    
    update_progress('comma_v0_1_training_dataset', count)
    print(f"Dataset comma_v0_1_training_dataset completed at {count}")


def procces_IndustryCorpus_news():
    ds = load_dataset("BAAI/IndustryCorpus_news", split="train", streaming=True)
    SAVE_PERIOD_TO_RAG = 512
    SAVE_PROGRESS_INTERVAL = 256
    progress_data = load_progress()
    offset = progress_data.get('IndustryCorpus_news', 0)
    print(f"\n{'='*60}")
    print(f"Processing: IndustryCorpus_news")
    print(f"Starting from offset: {offset}")
    print(f"{'='*60}\n")
    count = 0
    for item in iter(ds):
        count += 1
        if count <= offset:
            continue
        try:
            text = item['text']
            combined_text = f"{text}{END_TOKEN}"
            if count % SAVE_PERIOD_TO_RAG == 0:
                combined_text_rag = remove_think_tags(combined_text)
                result = embed_text_to_rag(combined_text_rag, chunk_size=1024, overlap=128)
            data = {
                'text': combined_text,
                'tokenized_text': tokenizer.encode(combined_text).ids,
                'category': GENERIC_TEXTS
            }
            url = "http://localhost:4567/save-data"
            response = session.post(url, json=data, timeout=5)
            if response.status_code != 200:
                print(f"Failed to save data: {response.text}")
            else:
                result = response.json()
                if count % 500 == 0:
                    pprint(result)
                    if 'processed_tokens' in result:
                        print(f"Processed: {format_tokens(result['processed_tokens'])} tokens")
                    wandb.log({
                        f"IndustryCorpus_news/file_counter": result.get('file_counter', 0),
                        f"IndustryCorpus_news/processed_tokens": result.get('processed_tokens', 0),
                        f"IndustryCorpus_news/requests_per_sec": result.get('requests_per_sec', 0),
                        f"IndustryCorpus_news/samples": count,
                    })
        except Exception as e:
            print(f"Error processing item {count}: {e}")
            continue
        if count % SAVE_PROGRESS_INTERVAL == 0:
            update_progress('IndustryCorpus_news', count)
            print(f"Progress saved: IndustryCorpus_news at {count}")    
    update_progress('IndustryCorpus_news', count)
    print(f"Dataset IndustryCorpus_news completed at {count}")

if __name__ == "__main__":
    wandb.init(
        project="portfolio-data-processing",
        name=f"data-pipeline-{int(time.time())}",
        config={
            "max_workers": 10,
            "connection_pool_size": 20,
        }
    )
    
    print("Running all dataset processors in parallel...")
    print(f"Using connection pooling with 20 max connections")
    
    with ThreadPoolExecutor(max_workers=64) as executor:
        futures = {
            executor.submit(process_am_if_r1_2pass): 'am_if_r1_2pass',
            executor.submit(process_am_if_r1_3pass): 'am_if_r1_3pass',
            executor.submit(process_am_code_r1_2pass): 'am_code_r1_2pass',
            executor.submit(process_am_code_r1_4pass): 'am_code_r1_4pass',
            executor.submit(process_am_math_r1_2pass): 'am_math_r1_2pass',
            executor.submit(process_am_math_r1_3pass): 'am_math_r1_3pass',
            executor.submit(process_am_math_r1_4pass): 'am_math_r1_4pass',
            executor.submit(process_am_code_r1_1pass): 'am_code_r1_1pass',
            executor.submit(process_am_code_7b_1pass): 'am_code_7b_1pass',
            executor.submit(process_am_code_7b_2pass): 'am_code_7b_2pass',
            executor.submit(process_am_code_7b_3pass): 'am_code_7b_3pass',
            executor.submit(process_am_code_7b_4pass): 'am_code_7b_4pass',
            executor.submit(process_am_if_7b_1pass): 'am_if_7b_1pass',
            executor.submit(process_am_if_7b_2pass): 'am_if_7b_2pass',
            executor.submit(process_am_if_7b_3pass): 'am_if_7b_3pass',
            executor.submit(process_am_if_7b_4pass): 'am_if_7b_4pass',
            executor.submit(process_am_if_r1_1pass): 'am_if_r1_1pass',
            executor.submit(process_am_if_r1_4pass): 'am_if_r1_4pass',
            executor.submit(process_am_math_7b_1pass): 'am_math_7b_1pass',
            executor.submit(process_am_math_7b_2pass): 'am_math_7b_2pass',
            executor.submit(process_am_math_7b_3pass): 'am_math_7b_3pass',
            executor.submit(process_am_math_7b_4pass): 'am_math_7b_4pass',
            executor.submit(process_nextcoder): 'nextcoder',
            executor.submit(process_codeforces_py): 'codeforces_py',
            executor.submit(process_codeforces): 'codeforces',
            executor.submit(process_opc_sft): 'opc_sft',
            executor.submit(process_codefeedback): 'codefeedback',
            executor.submit(process_leetcode): 'leetcode',
            executor.submit(process_math_datasets): 'math',
            executor.submit(process_general_datasets): 'general',
            executor.submit(process_stack_python): 'stack_python',
            executor.submit(process_stack_go): 'stack_go',
            executor.submit(process_stack_c): 'stack_c',
            executor.submit(process_stack_cpp): 'stack_cpp',
            executor.submit(process_stack_rust): 'stack_rust',
            executor.submit(process_stack_javascript): 'stack_js',
            executor.submit(process_stack_typescript): 'stack_ts',
            executor.submit(process_stack_sql): 'stack_sql',
            executor.submit(process_stack_assembly): 'stack_asm',
            executor.submit(process_synthetic_1): 'synthetic_1',
            executor.submit(process_s1k): 's1k',
            executor.submit(process_natural_reasoning): 'natural_reasoning',
            executor.submit(process_long): 'long',
            executor.submit(process_stackoverflow): 'stackoverflow',
            executor.submit(process_truthful_qa): 'truthful_qa',
            executor.submit(process_linux_commands): 'linux_commands',
            executor.submit(process_mmlu): 'mmlu',
            executor.submit(process_cosmo_1B_claude_taxonomy220): 'cosmo_1B_claude_taxonomy220',
            executor.submit(process_oh_dcft_v3_1_claude_3_5_haiku_20241022): 'oh_dcft_v3_1_claude_3_5_haiku_20241022',
            executor.submit(process_reason_code_search_net_python): 'reason_code_search_net_python',
            executor.submit(process_wiki_qa): 'wiki_qa',
            executor.submit(process_triviaqa): 'triviaqa',
            executor.submit(process_dolphin_r1): 'dolphin_r1',
            executor.submit(process_textbooks_are_all_you_need_lite): 'textbooks_are_all_you_need_lite',
            executor.submit(process_leet10k_alpaca): 'leet10k_alpaca',
            executor.submit(process_TextbookReasoning): 'TextbookReasoning',
            executor.submit(process_cosmopedia_v2_textbook_and_howto_8_3m): 'cosmopedia_v2_textbook_and_howto_8_3m',
            executor.submit(process_science_theory_textbooks): 'science_theory_textbooks',
            executor.submit(proccess_the_pile_github): 'the_pile_github',
            executor.submit(procces_ODA_Mixture_100k): 'ODA_Mixture_100k',
            executor.submit(process_golang_coder): 'golang_coder',
            executor.submit(process_LiteCoder_SFT_Terminal_preview): 'LiteCoder_SFT_Terminal_preview',
            executor.submit(process_dolphin_coder): 'dolphin_coder',
            executor.submit(proccess_textbook_codex): 'textbook_codex',
            executor.submit(process_OpenCodeReasoning): 'OpenCodeReasoning',
            executor.submit(process_pile): 'pile',
            executor.submit(proccess_MegaMath_Web_Pro_Max): 'MegaMath_Web_Pro_Max',
            executor.submit(proccess_FineFineWeb): 'FineFineWeb',
            executor.submit(proccess_pretraining_v1_omega_books): 'pretraining_v1_omega_books',
            executor.submit(proccess_c4): 'c4',
            executor.submit(process_reddit_dataset_157): 'reddit_dataset_157',
            executor.submit(procces_top_reddit_posts_daily): 'top_reddit_posts_daily',
            executor.submit(procces_codesearchnet_qa): 'codesearchnet_qa',
            executor.submit(procces_comma_v0_1_training_dataset): 'comma_v0_1_training_dataset',
            executor.submit(procces_IndustryCorpus_news): 'IndustryCorpus_news',
            executor.submit(proccess_chatbot_arena_conversations): 'chatbot_arena_conversations',
            executor.submit(process_reasoning_conversations_advanced_1m): 'reasoning_conversations_advanced_1m'
        }
        
        for future in as_completed(futures):
            name = futures[future]
            try:
                future.result()
                print(f"\n{'='*60}")
                print(f"{name} processing completed successfully")
                print(f"{'='*60}\n")
            except Exception as e:
                print(f"\n{'='*60}")
                print(f"{name} processing failed: {e}")
                print(f"{'='*60}\n")
    
    wandb.finish()
    print("All processing complete!")
