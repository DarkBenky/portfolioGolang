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
        batch_size=4,
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
        # ['math_r1_1pass', MATH_TEXTS],
        # ['code_r1_3pass', CODE_MIX_TEXTS],
        ['if_r1_2pass', GENERIC_TEXTS],
        ['if_r1_3pass', GENERIC_TEXTS],
        ['code_r1_2pass', CODE_MIX_TEXTS],
        ['code_r1_4pass', CODE_MIX_TEXTS],
        ['math_r1_2pass', MATH_TEXTS],
        ['math_r1_3pass', MATH_TEXTS],
        ['math_r1_4pass', MATH_TEXTS],
        ['code_r1_1pass', CODE_MIX_TEXTS],
        ['code_7b_1pass', CODE_MIX_TEXTS],
        ['code_7b_2pass', CODE_MIX_TEXTS],
        ['code_7b_3pass', CODE_MIX_TEXTS],
        ['code_7b_4pass', CODE_MIX_TEXTS],
        ['if_7b_1pass', GENERIC_TEXTS],
        ['if_7b_2pass', GENERIC_TEXTS],
        ['if_7b_3pass', GENERIC_TEXTS],
        ['if_7b_4pass', GENERIC_TEXTS],
        ['if_r1_1pass', GENERIC_TEXTS],
        ['if_r1_4pass', GENERIC_TEXTS],
        # ['code_1.5b_1pass', CODE_MIX_TEXTS],
        # ['code_1.5b_2pass', CODE_MIX_TEXTS],
        # ['code_1.5b_3pass', CODE_MIX_TEXTS],
        # ['code_1.5b_4pass', CODE_MIX_TEXTS],
        # ['if_1.5b_1pass', GENERIC_TEXTS],
        # ['if_1.5b_2pass', GENERIC_TEXTS],
        # ['if_1.5b_3pass', GENERIC_TEXTS],
        # ['if_1.5b_4pass', GENERIC_TEXTS],
        # ['math_1.5b_1pass', MATH_TEXTS],
        # ['math_1.5b_2pass', MATH_TEXTS],
        # ['math_1.5b_3pass', MATH_TEXTS],
        # ['math_1.5b_4pass', MATH_TEXTS],
        ['math_7b_1pass', MATH_TEXTS],
        ['math_7b_2pass', MATH_TEXTS],
        ['math_7b_3pass', MATH_TEXTS],
        ['math_7b_4pass', MATH_TEXTS],
    ]
    
    SAVE_PERIOD_TO_RAG = 16
    SAVE_PROGRESS_INTERVAL = 8192

    progress_data = load_progress()

    for split in splits:
        split_name = split[0]
        category = split[1]
        offset = progress_data.get(split_name, 0)

        print(f"\n{'='*60}")
        print(f"Processing split: {split_name}")
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
            combined_text = f"### User:\n{question}\n\n### Assistant:\n{answer}"

            try:
                if count % SAVE_PERIOD_TO_RAG == 0:
                    combined_text_rag = remove_think_tags(combined_text)
                    result = embed_text_to_rag(
                        combined_text_rag,
                        chunk_size=1024,
                        overlap=128
                    )
                
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

def process_code_datasets():
    datasets_config = [
        ['microsoft/NextCoderDataset', 'train', None, CODE_MIX_TEXTS, 'nextcoder'],
        ['open-r1/codeforces-cots', 'train', 'solutions_py', CODE_MIX_TEXTS, 'codeforces_py'],
        ['open-r1/codeforces-cots', 'train', 'solutions', CODE_MIX_TEXTS, 'codeforces'],
        ['OpenCoder-LLM/opc-sft-stage2', 'train', 'educational_instruct', CODE_MIX_TEXTS, 'opc_sft'],
        ['OpenCoder-LLM/CodeFeedback-Filtered-Instruction', 'train', None, CODE_MIX_TEXTS, 'codefeedback'],
        ['newfacade/LeetCodeDataset', 'train', None, CODE_MIX_TEXTS, 'leetcode'],
    ]
    
    SAVE_PERIOD_TO_RAG = 16
    SAVE_PROGRESS_INTERVAL = 4096
    
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
                    if 'nextcoder' in progress_key:
                        prompt = item['prompt']
                        completion = item['completion']
                        combined_text = f"### User:\n{prompt}\n### Assistant:\n{completion}{END_TOKEN}"
                    
                    elif 'codeforces' in progress_key:
                        messages = item['messages']
                        combined_text = ""
                        for msg in messages:
                            if msg['role'] == 'user':
                                combined_text += f"### User:\n{msg['content']}\n"
                            elif msg['role'] == 'assistant':
                                combined_text += f"### Assistant:\n{msg['content']}\n"
                        combined_text += END_TOKEN
                    
                    elif 'opc_sft' in progress_key:
                        instruction = item['instruction']
                        output = item['output']
                        combined_text = f"### Instruction:\n{instruction}\n### Output:\n{output}{END_TOKEN}"
                    
                    elif 'codefeedback' in progress_key:
                        instruction = item['query']
                        output = item['answer']
                        lang = item.get('lang', '')
                        combined_text = f"### Instruction:\n{instruction}\n### Language: {lang}\n### Output:\n{output}{END_TOKEN}"
                    
                    elif 'leetcode' in progress_key:
                        query = item['query']
                        response = item['response']
                        combined_text = f"{query}\n### Solution:\n{response}{END_TOKEN}"
                    
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

def process_math_datasets():
    datasets_config = [
        ['nvidia/OpenMathReasoning', None, None, MATH_TEXTS, 'openmathre asoning'],
        ['OpenDataArena/ODA-Math-460k', 'train', None, MATH_TEXTS, 'oda_math'],
    ]
    
    SAVE_PERIOD_TO_RAG = 24
    SAVE_PROGRESS_INTERVAL = 4096
    
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
    SAVE_PROGRESS_INTERVAL = 8192
    
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
    
    SAVE_PERIOD_TO_RAG = 16
    SAVE_PROGRESS_INTERVAL = 4096
    
    progress_data = load_progress()
    
    for lang_config in languages:
        lang, category, progress_key = lang_config
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
    
    with ThreadPoolExecutor(max_workers=10) as executor:
        futures = {
            executor.submit(process_am_deepseek_distilled_40m): 'deepseek',
            executor.submit(process_code_datasets): 'code',
            executor.submit(process_math_datasets): 'math',
            executor.submit(process_general_datasets): 'general',
            executor.submit(process_the_stack_datasets): 'stack',
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
