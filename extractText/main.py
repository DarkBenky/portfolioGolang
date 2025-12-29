# import os
# import time

# # MUST SET THESE FIRST before importing datasets
# os.environ['HF_HOME'] = '/media/user/2TB Clear/Data'
# os.environ['HF_DATASETS_CACHE'] = '/media/user/2TB Clear/Data/huggingface_cache'

# import sqlite3
# import json
# import pandas as pd
# from tokenizers import Tokenizer
# from tokenizers.models import Unigram
# from tokenizers.trainers import UnigramTrainer
# from tokenizers.pre_tokenizers import Whitespace
# from datasets import load_dataset

# MAX_COUNT = 10_000

# print("Cache directory:", os.environ['HF_DATASETS_CACHE'])
# print("Loading datasets...")

# # Load OpenThoughts dataset
# ds = load_dataset("open-thoughts/OpenThoughts3-1.2M")
# ds_train = ds['train']

# # Load Codeforces datasets
# ds_code = load_dataset("open-r1/codeforces-cots", "solutions")
# ds_code_train = ds_code['train']

# ds_code_py = load_dataset("open-r1/codeforces-cots", "solutions_py")
# ds_code_py_train = ds_code_py['train']

# # Load ArXiv summarization dataset
# ds_summarize = load_dataset("ccdv/arxiv-summarization", "section")
# ds_summarize_train = ds_summarize['train']

# # Load OpenMathReasoning dataset
# ds_maths = load_dataset("nvidia/OpenMathReasoning")
# print(f"OpenMathReasoning splits: {list(ds_maths.keys())}")
# # Use the first available split
# ds_maths_train = ds_maths[list(ds_maths.keys())[0]]

# # Load CSV files
# df = pd.read_csv('train.csv')
# df_python = pd.read_csv('ProblemSolutionPythonV3.csv')

# # Connect to database
# db = sqlite3.connect('../portfolio.db')
# cursor = db.cursor()

# END_TOKEN = '<EOS>'

# texts = []
# fullText = ""

# print("Processing news data...")
# try:
#     res = cursor.execute('SELECT summary, text FROM news').fetchall()
#     for row in res:
#         fullText += row[0] + " " + END_TOKEN + "\n" + row[1] + " " + END_TOKEN + "\n"
#         texts.append({'rawText': row[0] + " " + END_TOKEN})
#         texts.append({'rawText': row[1] + " " + END_TOKEN})
#     print(f"Added {len(res)*2} news texts")
# except Exception as e:
#     print(f"Error processing news: {e}")

# print("Processing sentiment data...")
# try:
#     res = cursor.execute('SELECT summary from daily_sentiment').fetchall()
#     for row in res:
#         fullText += row[0] + " " + END_TOKEN + "\n"
#         texts.append({'rawText': row[0] + " " + END_TOKEN})
#     print(f"Added {len(res)} daily sentiment texts")
# except Exception as e:
#     print(f"Error processing daily sentiment: {e}")

# try:
#     res = cursor.execute('SELECT summary from portfolio_daily_sentiment').fetchall()
#     for row in res:
#         fullText += row[0] + " " + END_TOKEN + "\n"
#         texts.append({'rawText': row[0] + " " + END_TOKEN})
#     print(f"Added {len(res)} portfolio sentiment texts")
# except Exception as e:
#     print(f"Error processing portfolio sentiment: {e}")

# print("Processing CSV prompts...")
# try:
#     for _, row in df.iterrows():
#         fullText += row['prompt'] + "\n" + END_TOKEN
#         texts.append({'rawText': row['prompt'] + " " + END_TOKEN})
#     print(f"Added {len(df)} CSV prompts")
# except Exception as e:
#     print(f"Error processing CSV: {e}")

# print("Processing Python problems...")
# try:
#     count = 0
#     for _, row in df_python.iterrows():
#         problem = str(row['Problem']) if pd.notna(row['Problem']) else ''
#         python_code = str(row['Python Code']) if pd.notna(row['Python Code']) else ''
        
#         if problem and python_code:
#             text_content = '### Instruction:\n' + problem + "\n" + '### Output:\n' + python_code + " " + END_TOKEN
#             fullText += text_content + "\n"
#             texts.append({'rawText': text_content})
#             count += 1
#     print(f"Added {count} Python problems")
# except Exception as e:
#     print(f"Error processing Python problems: {e}")

# print("Processing OpenThoughts dataset...")
# start = time.time()
# try:
#     count = 0
#     for i, item in enumerate(ds_train):
#         if i % 500 == 0 and i > 0:
#             processed = count
#             elapsed = time.time() - start
#             rps = (processed / elapsed) if elapsed > 0 else 0
#             print(f"Processed {processed} OpenThoughts items...")
#             print(f"Records per second: {rps:.2f}")
#             if rps > 0:
#                 remaining_sec = max(0, MAX_COUNT - processed) / rps
#                 print(f"Remaining approx. time: {remaining_sec/60:.2f} minutes")
#             else:
#                 print("Remaining approx. time: unknown (insufficient data)")

#         if count >= MAX_COUNT:
#             break
        
#         conversations = item['conversations']
        
#         # Add full conversation
#         conversation_text = ""
#         for turn in conversations:
#             role = turn['from'].capitalize()
#             if role == 'Gpt':
#                 role = 'Assistant'
#             conversation_text += "### " + role + ":\n" + turn['value'] + "\n"
#         conversation_text += END_TOKEN
#         fullText += conversation_text + "\n"
#         texts.append({'rawText': conversation_text})
#         count += 1
#     print(f"Added {count} OpenThoughts conversations")
# except Exception as e:
#     print(f"Error processing OpenThoughts: {e}")

# print("Processing Codeforces Python solutions...")
# try:
#     count = 0
#     start = time.time()
#     for i, item in enumerate(ds_code_py_train):
#         if i % 500 == 0 and i > 0:
#             processed = count
#             elapsed = time.time() - start
#             rps = (processed / elapsed) if elapsed > 0 else 0
#             print(f"Processed {processed} Codeforces Python items...")
#             print(f"Records per second: {rps:.2f}")
#             if rps > 0:
#                 remaining_sec = max(0, MAX_COUNT - processed) / rps
#                 print(f"Remaining approx. time: {remaining_sec/60:.2f} minutes")
#             else:
#                 print("Remaining approx. time: unknown (insufficient data)")

#         if count >= MAX_COUNT:
#             break
        
#         messages = item['messages']
#         code_text = ""
#         for msg in messages:
#             if msg['role'] == 'user':
#                 code_text += '### User:\n' + msg['content'] + "\n"
#             elif msg['role'] == 'assistant':
#                 code_text += '### Assistant:\n' + msg['content'] + "\n"
#         code_text += END_TOKEN
#         fullText += code_text + "\n"
#         texts.append({'rawText': code_text})
#         count += 1
#     print(f"Added {count} Codeforces Python solutions")
# except Exception as e:
#     print(f"Error processing Codeforces Python: {e}")

# print("Processing Codeforces general solutions...")
# try:
#     count = 0
#     start = time.time()
#     for i, item in enumerate(ds_code_train):
#         if i % 500 == 0 and i > 0:
#             processed = count
#             elapsed = time.time() - start
#             rps = (processed / elapsed) if elapsed > 0 else 0
#             print(f"Processed {processed} Codeforces general items...")
#             print(f"Records per second: {rps:.2f}")
#             if rps > 0:
#                 remaining_sec = max(0, MAX_COUNT - processed) / rps
#                 print(f"Remaining approx. time: {remaining_sec/60:.2f} minutes")
#             else:
#                 print("Remaining approx. time: unknown (insufficient data)")

#         if count >= MAX_COUNT:
#             break
        
#         messages = item['messages']
#         code_text = ""
#         for msg in messages:
#             if msg['role'] == 'user':
#                 code_text += '### User:\n' + msg['content'] + "\n"
#             elif msg['role'] == 'assistant':
#                 code_text += '### Assistant:\n' + msg['content'] + "\n"
#         code_text += END_TOKEN
#         fullText += code_text + "\n"
#         texts.append({'rawText': code_text})
#         count += 1
#     print(f"Added {count} Codeforces general solutions")
# except Exception as e:
#     print(f"Error processing Codeforces general: {e}")

# print("Processing OpenMathReasoning dataset...")
# try:
#     count = 0
#     start = time.time()
#     for i, item in enumerate(ds_maths_train):
#         if i % 500 == 0 and i > 0:
#             processed = count
#             elapsed = time.time() - start
#             rps = (processed / elapsed) if elapsed > 0 else 0
#             print(f"Processed {processed} math items...")
#             print(f"Records per second: {rps:.2f}")
#             if rps > 0:
#                 remaining_sec = max(0, MAX_COUNT - processed) / rps
#                 print(f"Remaining approx. time: {remaining_sec/60:.2f} minutes")
#             else:
#                 print("Remaining approx. time: unknown (insufficient data)")

#         if count >= MAX_COUNT:
#             break
        
#         problem = item['problem']
#         solution = item['generated_solution']
#         text_content = '### Problem:\n' + problem + "\n" + '### Solution:\n' + solution + " " + END_TOKEN
#         fullText += text_content + "\n"
#         texts.append({'rawText': text_content})
#         count += 1
#     print(f"Added {count} math problems")
# except Exception as e:
#     print(f"Error processing math dataset: {e}")

# print("Processing ArXiv summarization dataset...")
# try:
#     count = 0
#     start = time.time()
#     for i, item in enumerate(ds_summarize_train):
#         if i % 500 == 0 and i > 0:
#             processed = count
#             elapsed = time.time() - start
#             rps = (processed / elapsed) if elapsed > 0 else 0
#             print(f"Processed {processed} ArXiv items...")
#             print(f"Records per second: {rps:.2f}")
#             if rps > 0:
#                 remaining_sec = max(0, MAX_COUNT - processed) / rps
#                 print(f"Remaining approx. time: {remaining_sec/60:.2f} minutes")
#             else:
#                 print("Remaining approx. time: unknown (insufficient data)")

#         if count >= MAX_COUNT:
#             break
        
#         article = item['article']
#         abstract = item['abstract']
        
#         # Skip if article or abstract is too short or missing
#         if len(article) < 50 or len(abstract) < 20:
#             continue
            
#         text_content = '### Article:\n' + article + "\n" + '### Summary:\n' + abstract + " " + END_TOKEN
#         fullText += text_content + "\n"
#         texts.append({'rawText': text_content})
#         count += 1
#     print(f"Added {count} ArXiv summaries")
# except Exception as e:
#     print(f"Error processing ArXiv: {e}")

# print("Writing corpus to file...")
# with open('corpus.txt', 'w', encoding='utf-8') as f:
#     f.write(fullText)

# print("Initializing tokenizer...")
# tokenizer = Tokenizer(Unigram())
# tokenizer.pre_tokenizer = Whitespace()

# trainer = UnigramTrainer(
#     vocab_size=10000,
#     show_progress=True,
#     special_tokens=["[PAD]", "[UNK]", END_TOKEN]
# )

# print("Training tokenizer...")
# tokenizer.train(['corpus.txt'], trainer)

# print("Tokenizing texts...")
# for i, textObj in enumerate(texts):
#     if i % 500 == 0:
#         print(f"Tokenized {i}/{len(texts)} texts...")
#     encoding = tokenizer.encode(textObj['rawText'])
#     textObj['tokenizedText'] = encoding.ids

# print("Saving tokenized texts...")
# with open('extractedTexts.json', 'w', encoding='utf-8') as f:
#     json.dump(texts, f, indent=4)

# print("Saving tokenizer...")
# tokenizer.save('tokenizer.json')

# # Statistics
# max_len = max(len(textObj['tokenizedText']) for textObj in texts)
# avg_len = sum(len(textObj['tokenizedText']) for textObj in texts) / len(texts)
# min_len = min(len(textObj['tokenizedText']) for textObj in texts)

# print("\n" + "="*50)
# print("DATASET STATISTICS")
# print("="*50)
# print(f"Total number of texts: {len(texts):,}")
# print(f"Vocabulary size: {tokenizer.get_vocab_size():,}")
# print(f"Maximum tokenized length: {max_len:,}")
# print(f"Average tokenized length: {avg_len:.2f}")
# print(f"Minimum tokenized length: {min_len:,}")
# print("="*50)

# db.close()
# print("\nProcessing complete!")

import os
import time
import gc

# MUST SET THESE FIRST before importing datasets
os.environ['HF_HOME'] = '/media/user/2TB Clear/Data'
os.environ['HF_DATASETS_CACHE'] = '/media/user/2TB Clear/Data/huggingface_cache'

import sqlite3
import json
import pandas as pd
from tokenizers import Tokenizer
from tokenizers.models import Unigram
from tokenizers.trainers import UnigramTrainer
from tokenizers.pre_tokenizers import Whitespace
from datasets import load_dataset
from specialCharacters import END_TOKEN

MAX_COUNT = 100_000
BATCH_SIZE = 2000
TOKENIZE_BATCH_SIZE = 100  # Smaller batches for tokenization
WRITE_EVERY = 1000  # Write to disk every N items

print("Cache directory:", os.environ['HF_DATASETS_CACHE'])

# =============================
# HELPER FUNCTIONS
# =============================

def process_conversations_batch(batch):
    """Process a batch of OpenThoughts conversations."""
    results = []
    for item in batch:
        conversations = item['conversations']
        conversation_text = ""
        for turn in conversations:
            role = turn['from'].capitalize()
            if role == 'Gpt':
                role = 'Assistant'
            conversation_text += "### " + role + ":\n" + turn['value'] + "\n"
        conversation_text += END_TOKEN
        results.append(conversation_text)
    return results

def process_messages_batch(batch):
    """Process a batch of Codeforces solutions."""
    results = []
    for item in batch:
        messages = item['messages']
        code_text = ""
        for msg in messages:
            if msg['role'] == 'user':
                code_text += '### User:\n' + msg['content'] + "\n"
            elif msg['role'] == 'assistant':
                code_text += '### Assistant:\n' + msg['content'] + "\n"
        code_text += END_TOKEN
        results.append(code_text)
    return results

def process_math_batch(batch):
    """Process a batch of math problems."""
    results = []
    for item in batch:
        problem = item['problem']
        solution = item['generated_solution']
        text_content = '### Problem:\n' + problem + "\n" + '### Solution:\n' + solution + " " + END_TOKEN
        results.append(text_content)
    return results

def process_arxiv_batch(batch):
    """Process a batch of ArXiv articles."""
    results = []
    for item in batch:
        article = item['article']
        abstract = item['abstract']
        
        if len(article) < 50 or len(abstract) < 20:
            continue
            
        text_content = '### Article:\n' + article + "\n" + '### Summary:\n' + abstract + " " + END_TOKEN
        results.append(text_content)
    return results

def batch_iterator(dataset, batch_size, max_count):
    """Yield batches from dataset."""
    batch = []
    count = 0
    for item in dataset:
        if count >= max_count:
            break
        batch.append(item)
        count += 1
        if len(batch) >= batch_size:
            yield batch
            batch = []
    if batch:
        yield batch

def write_to_corpus(text_parts, corpus_file):
    """Append text parts to corpus file."""
    with open(corpus_file, 'a', encoding='utf-8') as f:
        for text in text_parts:
            f.write(text + "\n")

# =============================
# MAIN PROCESSING
# =============================

print("Loading datasets...")

ds = load_dataset("open-thoughts/OpenThoughts3-1.2M", streaming=False)
ds_train = ds['train']

ds_code = load_dataset("open-r1/codeforces-cots", "solutions", streaming=False)
ds_code_train = ds_code['train']

ds_code_py = load_dataset("open-r1/codeforces-cots", "solutions_py", streaming=False)
ds_code_py_train = ds_code_py['train']

ds_summarize = load_dataset("ccdv/arxiv-summarization", "section", streaming=False)
ds_summarize_train = ds_summarize['train']

ds_maths = load_dataset("nvidia/OpenMathReasoning", streaming=False)
print(f"OpenMathReasoning splits: {list(ds_maths.keys())}")
ds_maths_train = ds_maths[list(ds_maths.keys())[0]]

df = pd.read_csv('train.csv')
df_python = pd.read_csv('ProblemSolutionPythonV3.csv')

try:
    with open('math_samples.json', 'r', encoding='utf-8') as f:
        math_samples = json.load(f)
    print(f"Loaded {len(math_samples)} math samples from JSON")
except FileNotFoundError:
    print("Warning: math_samples.json not found, skipping...")
    math_samples = []
except Exception as e:
    print(f"Error loading math_samples.json: {e}")
    math_samples = []

db = sqlite3.connect('../portfolio.db')
cursor = db.cursor()

# Clear corpus file if it exists
corpus_file = 'corpus.txt'
if os.path.exists(corpus_file):
    os.remove(corpus_file)

print("Processing database data...")
text_buffer = []

try:
    res = cursor.execute('SELECT summary, text FROM news').fetchall()
    for row in res:
        text_buffer.append(row[0] + " " + END_TOKEN)
        text_buffer.append(row[1] + " " + END_TOKEN)
    print(f"Added {len(res)*2} news texts")
except Exception as e:
    print(f"Error processing news: {e}")

try:
    res = cursor.execute('SELECT summary from daily_sentiment').fetchall()
    for row in res:
        text_buffer.append(row[0] + " " + END_TOKEN)
    print(f"Added {len(res)} daily sentiment texts")
except Exception as e:
    print(f"Error processing daily sentiment: {e}")

try:
    res = cursor.execute('SELECT summary from portfolio_daily_sentiment').fetchall()
    for row in res:
        text_buffer.append(row[0] + " " + END_TOKEN)
    print(f"Added {len(res)} portfolio sentiment texts")
except Exception as e:
    print(f"Error processing portfolio sentiment: {e}")

print("Processing CSV prompts...")
try:
    for _, row in df.iterrows():
        text_buffer.append(row['prompt'] + "\n" + END_TOKEN)
    print(f"Added {len(df)} CSV prompts")
except Exception as e:
    print(f"Error processing CSV: {e}")

print("Processing Python problems...")
try:
    count = 0
    for _, row in df_python.iterrows():
        problem = str(row['Problem']) if pd.notna(row['Problem']) else ''
        python_code = str(row['Python Code']) if pd.notna(row['Python Code']) else ''
        
        if problem and python_code:
            text_content = '### Instruction:\n' + problem + "\n" + '### Output:\n' + python_code + " " + END_TOKEN
            text_buffer.append(text_content)
            count += 1
    print(f"Added {count} Python problems")
except Exception as e:
    print(f"Error processing Python problems: {e}")

print("Processing math samples from JSON...")
try:
    count = 0
    for sample in math_samples:
        prompt = sample.get('prompt', '')
        response = sample.get('response', '')
        difficulty = sample.get('difficulty', 'unknown')
        
        if prompt and response:
            text_content = f"### Math Problem ({difficulty}):\n{prompt}\n### Solution:\n{response} {END_TOKEN}"
            text_buffer.append(text_content)
            count += 1
    print(f"Added {count} math samples")
except Exception as e:
    print(f"Error processing math samples: {e}")

# Write initial buffer
write_to_corpus(text_buffer, corpus_file)
total_texts = len(text_buffer)
del text_buffer
gc.collect()

# Process large datasets and write incrementally
print("Processing OpenThoughts dataset...")
start = time.time()
try:
    count = 0
    for batch in batch_iterator(ds_train, BATCH_SIZE, MAX_COUNT):
        batch_results = process_conversations_batch(batch)
        write_to_corpus(batch_results, corpus_file)
        count += len(batch_results)
        total_texts += len(batch_results)
        
        if count % 5000 == 0:
            elapsed = time.time() - start
            rps = count / elapsed if elapsed > 0 else 0
            print(f"Processed {count} OpenThoughts items (RPS: {rps:.2f})")
            gc.collect()
    
    print(f"Added {count} OpenThoughts conversations")
except Exception as e:
    print(f"Error processing OpenThoughts: {e}")

print("Processing Codeforces Python solutions...")
start = time.time()
try:
    count = 0
    for batch in batch_iterator(ds_code_py_train, BATCH_SIZE, MAX_COUNT):
        batch_results = process_messages_batch(batch)
        write_to_corpus(batch_results, corpus_file)
        count += len(batch_results)
        total_texts += len(batch_results)
        
        if count % 5000 == 0:
            elapsed = time.time() - start
            rps = count / elapsed if elapsed > 0 else 0
            print(f"Processed {count} Codeforces Python items (RPS: {rps:.2f})")
            gc.collect()
    
    print(f"Added {count} Codeforces Python solutions")
except Exception as e:
    print(f"Error processing Codeforces Python: {e}")

print("Processing Codeforces general solutions...")
start = time.time()
try:
    count = 0
    for batch in batch_iterator(ds_code_train, BATCH_SIZE, MAX_COUNT):
        batch_results = process_messages_batch(batch)
        write_to_corpus(batch_results, corpus_file)
        count += len(batch_results)
        total_texts += len(batch_results)
        
        if count % 5000 == 0:
            elapsed = time.time() - start
            rps = count / elapsed if elapsed > 0 else 0
            print(f"Processed {count} Codeforces general items (RPS: {rps:.2f})")
            gc.collect()
    
    print(f"Added {count} Codeforces general solutions")
except Exception as e:
    print(f"Error processing Codeforces general: {e}")

print("Processing OpenMathReasoning dataset...")
start = time.time()
try:
    count = 0
    for batch in batch_iterator(ds_maths_train, BATCH_SIZE, MAX_COUNT):
        batch_results = process_math_batch(batch)
        write_to_corpus(batch_results, corpus_file)
        count += len(batch_results)
        total_texts += len(batch_results)
        
        if count % 5000 == 0:
            elapsed = time.time() - start
            rps = count / elapsed if elapsed > 0 else 0
            print(f"Processed {count} math items (RPS: {rps:.2f})")
            gc.collect()
    
    print(f"Added {count} math problems")
except Exception as e:
    print(f"Error processing math dataset: {e}")

print("Processing ArXiv summarization dataset...")
start = time.time()
try:
    count = 0
    for batch in batch_iterator(ds_summarize_train, BATCH_SIZE, MAX_COUNT):
        batch_results = process_arxiv_batch(batch)
        write_to_corpus(batch_results, corpus_file)
        count += len(batch_results)
        total_texts += len(batch_results)
        
        if count % 5000 == 0:
            elapsed = time.time() - start
            rps = count / elapsed if elapsed > 0 else 0
            print(f"Processed {count} ArXiv items (RPS: {rps:.2f})")
            gc.collect()
    
    print(f"Added {count} ArXiv summaries")
except Exception as e:
    print(f"Error processing ArXiv: {e}")

print("Initializing tokenizer...")
tokenizer = Tokenizer(Unigram())
tokenizer.pre_tokenizer = Whitespace()

trainer = UnigramTrainer(
    vocab_size=32000,
    show_progress=True,
    special_tokens=["[PAD]", "[UNK]", END_TOKEN]
)

print("Training tokenizer...")
tokenizer.train([corpus_file], trainer)

print("Saving tokenizer...")
tokenizer.save('tokenizer.json')

# Stream tokenization - read corpus line by line and tokenize
print("Tokenizing texts (streaming mode)...")

output_file = 'extractedTexts.json'
if os.path.exists(output_file):
    os.remove(output_file)

tokenized_texts = []
token_lengths = []
processed = 0
start = time.time()

# Open output file for writing
with open(corpus_file, 'r', encoding='utf-8') as f:
    batch = []
    
    for line in f:
        line = line.strip()
        if not line:
            continue
            
        batch.append(line)
        
        if len(batch) >= TOKENIZE_BATCH_SIZE:
            # Tokenize batch
            for text in batch:
                encoding = tokenizer.encode(text)
                tokenized_texts.append({
                    'rawText': text,
                    'tokenizedText': encoding.ids
                })
                token_lengths.append(len(encoding.ids))
                processed += 1
            
            # Write to disk periodically
            if processed % WRITE_EVERY == 0:
                # Append to JSON file
                if processed == WRITE_EVERY:
                    # First write - create array
                    with open(output_file, 'w', encoding='utf-8') as out_f:
                        json.dump(tokenized_texts, out_f, indent=4)
                else:
                    # Subsequent writes - need to merge
                    with open(output_file, 'r', encoding='utf-8') as out_f:
                        existing = json.load(out_f)
                    existing.extend(tokenized_texts)
                    with open(output_file, 'w', encoding='utf-8') as out_f:
                        json.dump(existing, out_f, indent=4)
                
                elapsed = time.time() - start
                rps = processed / elapsed if elapsed > 0 else 0
                print(f"Tokenized {processed}/{total_texts} texts (RPS: {rps:.2f})")
                
                tokenized_texts = []
                gc.collect()
            
            batch = []
    
    # Process remaining batch
    if batch:
        for text in batch:
            encoding = tokenizer.encode(text)
            tokenized_texts.append({
                'rawText': text,
                'tokenizedText': encoding.ids
            })
            token_lengths.append(len(encoding.ids))
            processed += 1

# Write final batch
if tokenized_texts:
    if os.path.exists(output_file):
        with open(output_file, 'r', encoding='utf-8') as out_f:
            existing = json.load(out_f)
        existing.extend(tokenized_texts)
        with open(output_file, 'w', encoding='utf-8') as out_f:
            json.dump(existing, out_f, indent=4)
    else:
        with open(output_file, 'w', encoding='utf-8') as out_f:
            json.dump(tokenized_texts, out_f, indent=4)

print(f"Tokenized {processed} texts total")

# Statistics
if token_lengths:
    max_len = max(token_lengths)
    avg_len = sum(token_lengths) / len(token_lengths)
    min_len = min(token_lengths)

    print("\n" + "="*50)
    print("DATASET STATISTICS")
    print("="*50)
    print(f"Total number of texts: {processed:,}")
    print(f"Vocabulary size: {tokenizer.get_vocab_size():,}")
    print(f"Maximum tokenized length: {max_len:,}")
    print(f"Average tokenized length: {avg_len:.2f}")
    print(f"Minimum tokenized length: {min_len:,}")
    print("="*50)

db.close()
print("\nProcessing complete!")