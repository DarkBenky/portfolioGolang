import os

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

print("Cache directory:", os.environ['HF_DATASETS_CACHE'])
print("Loading datasets...")

# Load OpenThoughts dataset
ds = load_dataset("open-thoughts/OpenThoughts3-1.2M")
ds_train = ds['train']

# Load Codeforces datasets
ds_code = load_dataset("open-r1/codeforces-cots", "solutions")
ds_code_train = ds_code['train']

ds_code_py = load_dataset("open-r1/codeforces-cots", "solutions_py")
ds_code_py_train = ds_code_py['train']

# Load ArXiv summarization dataset
ds_summarize = load_dataset("ccdv/arxiv-summarization", "section")
ds_summarize_train = ds_summarize['train']

# Load OpenMathReasoning dataset
ds_maths = load_dataset("nvidia/OpenMathReasoning")
print(f"OpenMathReasoning splits: {list(ds_maths.keys())}")
# Use the first available split
ds_maths_train = ds_maths[list(ds_maths.keys())[0]]

# Load CSV files
df = pd.read_csv('train.csv')
df_python = pd.read_csv('ProblemSolutionPythonV3.csv')

# Connect to database
db = sqlite3.connect('../portfolio.db')
cursor = db.cursor()

END_TOKEN = '<EOS>'

texts = []
fullText = ""

print("Processing news data...")
try:
    res = cursor.execute('SELECT summary, text FROM news').fetchall()
    for row in res:
        fullText += row[0] + " " + END_TOKEN + "\n" + row[1] + " " + END_TOKEN + "\n"
        texts.append({'rawText': row[0] + " " + END_TOKEN})
        texts.append({'rawText': row[1] + " " + END_TOKEN})
    print(f"Added {len(res)*2} news texts")
except Exception as e:
    print(f"Error processing news: {e}")

print("Processing sentiment data...")
try:
    res = cursor.execute('SELECT summary from daily_sentiment').fetchall()
    for row in res:
        fullText += row[0] + " " + END_TOKEN + "\n"
        texts.append({'rawText': row[0] + " " + END_TOKEN})
    print(f"Added {len(res)} daily sentiment texts")
except Exception as e:
    print(f"Error processing daily sentiment: {e}")

try:
    res = cursor.execute('SELECT summary from portfolio_daily_sentiment').fetchall()
    for row in res:
        fullText += row[0] + " " + END_TOKEN + "\n"
        texts.append({'rawText': row[0] + " " + END_TOKEN})
    print(f"Added {len(res)} portfolio sentiment texts")
except Exception as e:
    print(f"Error processing portfolio sentiment: {e}")

print("Processing CSV prompts...")
try:
    for _, row in df.iterrows():
        fullText += row['prompt'] + "\n" + END_TOKEN
        texts.append({'rawText': row['prompt'] + " " + END_TOKEN})
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
            fullText += text_content + "\n"
            texts.append({'rawText': text_content})
            count += 1
    print(f"Added {count} Python problems")
except Exception as e:
    print(f"Error processing Python problems: {e}")

print("Processing OpenThoughts dataset...")
try:
    count = 0
    for i, item in enumerate(ds_train):
        if i % 100000 == 0 and i > 0:
            print(f"Processed {i} OpenThoughts items...")
        
        conversations = item['conversations']
        
        # Add full conversation
        conversation_text = ""
        for turn in conversations:
            role = turn['from'].capitalize()
            if role == 'Gpt':
                role = 'Assistant'
            conversation_text += "### " + role + ":\n" + turn['value'] + "\n"
        conversation_text += END_TOKEN
        fullText += conversation_text + "\n"
        texts.append({'rawText': conversation_text})
        count += 1
    print(f"Added {count} OpenThoughts conversations")
except Exception as e:
    print(f"Error processing OpenThoughts: {e}")

print("Processing Codeforces Python solutions...")
try:
    count = 0
    for i, item in enumerate(ds_code_py_train):
        if i % 10000 == 0 and i > 0:
            print(f"Processed {i} Codeforces Python items...")
        
        messages = item['messages']
        code_text = ""
        for msg in messages:
            if msg['role'] == 'user':
                code_text += '### User:\n' + msg['content'] + "\n"
            elif msg['role'] == 'assistant':
                code_text += '### Assistant:\n' + msg['content'] + "\n"
        code_text += END_TOKEN
        fullText += code_text + "\n"
        texts.append({'rawText': code_text})
        count += 1
    print(f"Added {count} Codeforces Python solutions")
except Exception as e:
    print(f"Error processing Codeforces Python: {e}")

print("Processing Codeforces general solutions...")
try:
    count = 0
    for i, item in enumerate(ds_code_train):
        if i % 10000 == 0 and i > 0:
            print(f"Processed {i} Codeforces general items...")
        
        messages = item['messages']
        code_text = ""
        for msg in messages:
            if msg['role'] == 'user':
                code_text += '### User:\n' + msg['content'] + "\n"
            elif msg['role'] == 'assistant':
                code_text += '### Assistant:\n' + msg['content'] + "\n"
        code_text += END_TOKEN
        fullText += code_text + "\n"
        texts.append({'rawText': code_text})
        count += 1
    print(f"Added {count} Codeforces general solutions")
except Exception as e:
    print(f"Error processing Codeforces general: {e}")

print("Processing OpenMathReasoning dataset...")
try:
    count = 0
    for i, item in enumerate(ds_maths_train):
        if i % 50000 == 0 and i > 0:
            print(f"Processed {i} math items...")
        
        problem = item['problem']
        solution = item['generated_solution']
        text_content = '### Problem:\n' + problem + "\n" + '### Solution:\n' + solution + " " + END_TOKEN
        fullText += text_content + "\n"
        texts.append({'rawText': text_content})
        count += 1
    print(f"Added {count} math problems")
except Exception as e:
    print(f"Error processing math dataset: {e}")

print("Processing ArXiv summarization dataset...")
try:
    count = 0
    for i, item in enumerate(ds_summarize_train):
        if i % 50000 == 0 and i > 0:
            print(f"Processed {i} ArXiv items...")
        
        article = item['article']
        abstract = item['abstract']
        
        # Skip if article or abstract is too short or missing
        if len(article) < 50 or len(abstract) < 20:
            continue
            
        text_content = '### Article:\n' + article + "\n" + '### Summary:\n' + abstract + " " + END_TOKEN
        fullText += text_content + "\n"
        texts.append({'rawText': text_content})
        count += 1
    print(f"Added {count} ArXiv summaries")
except Exception as e:
    print(f"Error processing ArXiv: {e}")

print("Writing corpus to file...")
with open('corpus.txt', 'w', encoding='utf-8') as f:
    f.write(fullText)

print("Initializing tokenizer...")
tokenizer = Tokenizer(Unigram())
tokenizer.pre_tokenizer = Whitespace()

trainer = UnigramTrainer(
    vocab_size=10000,
    show_progress=True,
    special_tokens=["[PAD]", "[UNK]", END_TOKEN]
)

print("Training tokenizer...")
tokenizer.train(['corpus.txt'], trainer)

print("Tokenizing texts...")
for i, textObj in enumerate(texts):
    if i % 50000 == 0:
        print(f"Tokenized {i}/{len(texts)} texts...")
    encoding = tokenizer.encode(textObj['rawText'])
    textObj['tokenizedText'] = encoding.ids

print("Saving tokenized texts...")
with open('extractedTexts.json', 'w', encoding='utf-8') as f:
    json.dump(texts, f, indent=4)

print("Saving tokenizer...")
tokenizer.save('tokenizer.json')

# Statistics
max_len = max(len(textObj['tokenizedText']) for textObj in texts)
avg_len = sum(len(textObj['tokenizedText']) for textObj in texts) / len(texts)
min_len = min(len(textObj['tokenizedText']) for textObj in texts)

print("\n" + "="*50)
print("DATASET STATISTICS")
print("="*50)
print(f"Total number of texts: {len(texts):,}")
print(f"Vocabulary size: {tokenizer.get_vocab_size():,}")
print(f"Maximum tokenized length: {max_len:,}")
print(f"Average tokenized length: {avg_len:.2f}")
print(f"Minimum tokenized length: {min_len:,}")
print("="*50)

db.close()
print("\nProcessing complete!")