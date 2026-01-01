import json
import pandas as pd
from tokenizers import Tokenizer
from ollama import chat
from specialCharacters import END_TOKEN
from pprint import pprint
import re
import os

# load tokenizer
tokenizer = Tokenizer.from_file('tokenizer.json')
vocab_size = tokenizer.get_vocab_size()

py_ds = pd.read_csv('train.csv')
leetcode_ds = pd.read_csv('leetcode.csv')

SAVE_PATH = 'extractedTexts_multilang.json'
CHECKPOINT_PATH = 'checkpoint.json'

data = []

LIMIT = None
SAVE_INTERVAL = 5
BATCH_SIZE = 5  # Number of rows to process from each dataset before switching

LANGUAGES = ['Python', 'JavaScript', 'C', 'Golang', 'Rust']

system_prompt = (
    "You are a coding assistant.\n"
    "Output ONLY valid code or plain ASCII text.\n"
    "Do NOT use emojis.\n"
    "Do NOT use special characters.\n"
    "Allowed characters: a-z A-Z 0-9 _ . , : ; () [] {} +-*/=<>'\" \\n \\t\n"
    "Do NOT add explanations unless explicitly requested.\n"
)

ALLOWED_PATTERN = re.compile(r"[^\x09\x0A\x0D\x20-\x7E]")

def sanitize_output(text: str) -> str:
    text = ALLOWED_PATTERN.sub("", text)
    return text

def load_existing_data():
    if os.path.exists(SAVE_PATH):
        try:
            with open(SAVE_PATH, 'r', encoding='utf-8') as f:
                return json.load(f)
        except:
            return []
    return []

def save_data(data_to_save):
    with open(SAVE_PATH, 'w', encoding='utf-8') as f:
        json.dump(data_to_save, f, ensure_ascii=False, indent=4)
    print(f"Saved {len(data_to_save)} entries to {SAVE_PATH}")

def save_checkpoint(dataset_type, index, language_index, train_position, leetcode_position):
    checkpoint = {
        'dataset_type': dataset_type,
        'last_row': index,
        'last_language_index': language_index,
        'train_position': train_position,
        'leetcode_position': leetcode_position
    }
    with open(CHECKPOINT_PATH, 'w', encoding='utf-8') as f:
        json.dump(checkpoint, f)

def load_checkpoint():
    if os.path.exists(CHECKPOINT_PATH):
        try:
            with open(CHECKPOINT_PATH, 'r', encoding='utf-8') as f:
                return json.load(f)
        except:
            return None
    return None

def remove_language_references(text: str) -> str:
    patterns = [
        r'\bPython\b',
        r'\bJavaScript\b',
        r'\bJS\b',
        r'\bC\+\+\b',
        r'\bC\b(?!\w)',
        r'\bGolang\b',
        r'\bGo\b(?=\s+code|\s+program|\s+function)',
        r'\bRust\b',
        r'\bJava\b',
        r'\bRuby\b',
        r'\bPHP\b',
        r'\bin\s+Python',
        r'\busing\s+Python',
        r'\bPython\s+code',
        r'\bPython\s+function',
        r'\bPython\s+program',
    ]
    
    cleaned_text = text
    for pattern in patterns:
        cleaned_text = re.sub(pattern, '', cleaned_text, flags=re.IGNORECASE)
    
    cleaned_text = re.sub(r'\s+', ' ', cleaned_text).strip()
    
    return cleaned_text

def process_train_row(row, language):
    original_instruction = row['instruction']
    input_text = row['input']
    
    cleaned_instruction = remove_language_references(original_instruction)
    lang_instruction = f"{cleaned_instruction} (Write the solution in {language})"
    
    result = chat(
        model="deepseek-r1:32b",
        messages=[
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": f"Instruction: {lang_instruction}\nInput: {input_text}"}
        ],
        options={
            "temperature": 0.1
        }
    )
    
    content = sanitize_output(result["message"]["content"])
    thinking = sanitize_output(result["message"].get("thinking", ""))
    
    full_response = f"""### Instruction:
{cleaned_instruction}

### Input:
{input_text}

### Target Language:
{language}

### Thinking:
{thinking}

### Response ({language}):
{content}
{END_TOKEN}"""
    
    return {
        "dataset": "train",
        "language": language,
        "original_instruction": original_instruction,
        "cleaned_instruction": cleaned_instruction,
        "rawText": full_response,
        "tokenizedText": tokenizer.encode(full_response).ids
    }

def process_leetcode_row(row, language):
    title = row['title']
    description = row['description']
    difficulty = row['difficulty']
    
    cleaned_description = remove_language_references(description)
    
    prompt = f"LeetCode Problem: {title}\nDifficulty: {difficulty}\n\n{cleaned_description}\n\nWrite a solution in {language}."
    
    result = chat(
        model="deepseek-r1:32b",
        messages=[
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": prompt}
        ],
        options={
            "temperature": 0.1
        }
    )
    
    content = sanitize_output(result["message"]["content"])
    thinking = sanitize_output(result["message"].get("thinking", ""))
    
    full_response = f"""### Problem:
{title} ({difficulty})

### Description:
{cleaned_description}

### Target Language:
{language}

### Thinking:
{thinking}

### Solution ({language}):
{content}
{END_TOKEN}"""
    
    return {
        "dataset": "leetcode",
        "language": language,
        "title": title,
        "difficulty": difficulty,
        "rawText": full_response,
        "tokenizedText": tokenizer.encode(full_response).ids
    }

data = load_existing_data()
print(f"Loaded {len(data)} existing entries")

checkpoint = load_checkpoint()
train_position = 0
leetcode_position = 0
current_dataset = 'train'
start_index = 0
start_language_index = 0

if checkpoint:
    current_dataset = checkpoint.get('dataset_type', 'train')
    start_index = checkpoint['last_row']
    start_language_index = checkpoint['last_language_index'] + 1
    train_position = checkpoint.get('train_position', 0)
    leetcode_position = checkpoint.get('leetcode_position', 0)
    
    if start_language_index >= len(LANGUAGES):
        start_index += 1
        start_language_index = 0
        
        if current_dataset == 'train':
            train_position = start_index
        else:
            leetcode_position = start_index
    
    print(f"Resuming from {current_dataset} dataset")
    print(f"Train position: {train_position}, Leetcode position: {leetcode_position}")
    print(f"Current row: {start_index}, language index: {start_language_index}")

entries_since_last_save = 0
train_len = len(py_ds)
leetcode_len = len(leetcode_ds)

while train_position < train_len or leetcode_position < leetcode_len:
    if LIMIT and (train_position >= LIMIT and leetcode_position >= LIMIT):
        break
    
    # Process train.csv batch
    if train_position < train_len:
        print(f"\n--- Processing TRAIN batch (rows {train_position} to {min(train_position + BATCH_SIZE, train_len)}) ---")
        
        batch_end = min(train_position + BATCH_SIZE, train_len)
        for index in range(train_position, batch_end):
            if LIMIT and index >= LIMIT:
                break
            
            row = py_ds.iloc[index]
            print(f"Processing train.csv row {index+1}/{train_len}")
            
            try:
                lang_start = start_language_index if (current_dataset == 'train' and index == start_index) else 0
                
                for lang_index, language in enumerate(LANGUAGES[lang_start:], start=lang_start):
                    entry = process_train_row(row, language)
                    
                    print(f'\n{"="*60}')
                    print(f'Dataset: train.csv | Row: {index+1} | Language: {language}')
                    print(f'{"="*60}')
                    print(entry['rawText'])
                    print(f'{"="*60}\n')
                    
                    data.append(entry)
                    entries_since_last_save += 1
                    
                    save_checkpoint('train', index, lang_index, train_position, leetcode_position)
                    
                    if entries_since_last_save >= SAVE_INTERVAL:
                        save_data(data)
                        entries_since_last_save = 0
                
                start_language_index = 0
                
            except Exception as e:
                print(f"Error processing train.csv row {index}: {e}")
                continue
        
        train_position = batch_end
        current_dataset = 'leetcode'
        start_index = leetcode_position
    
    # Process leetcode.csv batch
    if leetcode_position < leetcode_len:
        print(f"\n--- Processing LEETCODE batch (rows {leetcode_position} to {min(leetcode_position + BATCH_SIZE, leetcode_len)}) ---")
        
        batch_end = min(leetcode_position + BATCH_SIZE, leetcode_len)
        for index in range(leetcode_position, batch_end):
            if LIMIT and index >= LIMIT:
                break
            
            row = leetcode_ds.iloc[index]
            print(f"Processing leetcode.csv row {index+1}/{leetcode_len}")
            
            try:
                lang_start = start_language_index if (current_dataset == 'leetcode' and index == start_index) else 0
                
                for lang_index, language in enumerate(LANGUAGES[lang_start:], start=lang_start):
                    entry = process_leetcode_row(row, language)
                    
                    print(f'\n{"="*60}')
                    print(f'Dataset: leetcode.csv | Row: {index+1} | Language: {language}')
                    print(f'{"="*60}')
                    print(entry['rawText'])
                    print(f'{"="*60}\n')
                    
                    data.append(entry)
                    entries_since_last_save += 1
                    
                    save_checkpoint('leetcode', index, lang_index, train_position, leetcode_position)
                    
                    if entries_since_last_save >= SAVE_INTERVAL:
                        save_data(data)
                        entries_since_last_save = 0
                
                start_language_index = 0
                
            except Exception as e:
                print(f"Error processing leetcode.csv row {index}: {e}")
                continue
        
        leetcode_position = batch_end
        current_dataset = 'train'
        start_index = train_position

save_data(data)

if os.path.exists(CHECKPOINT_PATH):
    os.remove(CHECKPOINT_PATH)
    print("Checkpoint file removed - processing complete")