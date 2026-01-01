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

SAVE_PATH = 'extractedTexts_multilang.json'
CHECKPOINT_PATH = 'checkpoint.json'

data = []

LIMIT = 3
SAVE_INTERVAL = 5  # Save every 5 successful entries

LANGUAGES = ['Python', 'JavaScript', 'C', 'Golang', 'Rust']

length = len(py_ds)

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
    """Load existing data if the file exists"""
    if os.path.exists(SAVE_PATH):
        try:
            with open(SAVE_PATH, 'r', encoding='utf-8') as f:
                return json.load(f)
        except:
            return []
    return []

def save_data(data_to_save):
    """Save data to file"""
    with open(SAVE_PATH, 'w', encoding='utf-8') as f:
        json.dump(data_to_save, f, ensure_ascii=False, indent=4)
    print(f"Saved {len(data_to_save)} entries to {SAVE_PATH}")

def save_checkpoint(index, language_index):
    """Save checkpoint information"""
    checkpoint = {
        'last_row': index,
        'last_language_index': language_index
    }
    with open(CHECKPOINT_PATH, 'w', encoding='utf-8') as f:
        json.dump(checkpoint, f)

def load_checkpoint():
    """Load checkpoint if exists"""
    if os.path.exists(CHECKPOINT_PATH):
        try:
            with open(CHECKPOINT_PATH, 'r', encoding='utf-8') as f:
                return json.load(f)
        except:
            return None
    return None

def remove_language_references(text: str) -> str:
    """Remove language-specific references from instruction text"""
    # Remove common language references (case-insensitive)
    patterns = [
        r'\bPython\b',
        r'\bJavaScript\b',
        r'\bJS\b',
        r'\bC\+\+\b',
        r'\bC\b(?!\w)',  # Match 'C' but not part of other words
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
    
    # Clean up extra whitespace
    cleaned_text = re.sub(r'\s+', ' ', cleaned_text).strip()
    
    return cleaned_text

# Load existing data
data = load_existing_data()
print(f"Loaded {len(data)} existing entries")

# Load checkpoint
checkpoint = load_checkpoint()
start_index = 0
start_language_index = 0

if checkpoint:
    start_index = checkpoint['last_row']
    start_language_index = checkpoint['last_language_index'] + 1
    if start_language_index >= len(LANGUAGES):
        start_index += 1
        start_language_index = 0
    print(f"Resuming from row {start_index + 1}, language index {start_language_index}")

entries_since_last_save = 0

for index, row in py_ds.iterrows():
    if index < start_index:
        continue
    
    print(f"Processing row {index+1}/{length}")
    
    try:
        original_instruction = row['instruction']
        input_text = row['input']

        if LIMIT and index >= LIMIT:
            break

        lang_start = start_language_index if index == start_index else 0
        
        for lang_index, language in enumerate(LANGUAGES[lang_start:], start=lang_start):
            # Remove language-specific references from the instruction
            cleaned_instruction = remove_language_references(original_instruction)
            
            # Create language-specific instruction
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
            
            content = result["message"]["content"]
            content = sanitize_output(content)

            thinking = result["message"].get("thinking", "")
            thinking = sanitize_output(thinking)

            # Create the formatted response with emphasis on the target language
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

            print(f'\n{"="*60}')
            print(f'Language: {language}')
            print(f'{"="*60}')
            print(full_response)
            print(f'{"="*60}\n')

            tokenized = tokenizer.encode(full_response).ids

            data.append(
                {
                    "language": language,
                    "original_instruction": original_instruction,
                    "cleaned_instruction": cleaned_instruction,
                    "rawText": full_response,
                    "tokenizedText": tokenized
                }
            )
            
            entries_since_last_save += 1
            
            # Save checkpoint after each language
            save_checkpoint(index, lang_index)
            
            # Periodic save
            if entries_since_last_save >= SAVE_INTERVAL:
                save_data(data)
                entries_since_last_save = 0
                
    except Exception as e:
        print(f"Error processing row {index}: {e}")
        continue

# Final save
save_data(data)

# Clean up checkpoint file
if os.path.exists(CHECKPOINT_PATH):
    os.remove(CHECKPOINT_PATH)
    print("Checkpoint file removed - processing complete")