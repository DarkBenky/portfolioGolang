from tokenizers import Tokenizer
from specialCharacters import END_TOKEN
import json
import os
import pandas as pd
import sqlite3

os.environ['HF_HOME'] = '/media/user/2TB Clear/Data'
os.environ['HF_DATASETS_CACHE'] = '/media/user/2TB Clear/Data/huggingface_cache'

from datasets import load_dataset

# Load tokenizer
tokenizer = Tokenizer.from_file('tokenizer.json')
print("Vocabulary size:", tokenizer.get_vocab_size())

output_file = 'extractedTexts.json'
temp_file = 'extractedTexts_temp.json'

# Collection for all new data
all_new_data = []

# =============================
# 1. NextCoder Dataset
# =============================
print("\n=== Processing NextCoder Dataset ===")
try:
    ds_code = load_dataset("microsoft/NextCoderDataset", split="train")
    l = len(ds_code)
    
    for i, sample in enumerate(ds_code):
        if i % 1000 == 0:
            print(f"NextCoder: {i}/{l}")
        
        try:
            prompt = sample['prompt']
            completion = sample['completion']
            text = '### User:\n' + prompt + '\n### Assistant:\n' + completion + END_TOKEN
            encoded = tokenizer.encode(text)
            
            all_new_data.append({
                "rawText": text,
                "tokenizedText": encoded.ids
            })
        except Exception as e:
            print(f"Error processing NextCoder sample {i}: {e}")
            continue
    
    print(f"Successfully processed {len(all_new_data)} NextCoder samples")
except Exception as e:
    print(f"Error loading NextCoder dataset: {e}")

# =============================
# 2. OpenThoughts Dataset
# =============================
print("\n=== Processing OpenThoughts Dataset ===")
try:
    ds = load_dataset("open-thoughts/OpenThoughts3-1.2M")
    ds_train = ds['train']
    l = len(ds_train)
    count = 0
    MAX_COUNT = 5_000
    
    for i, item in enumerate(ds_train):
        if count >= MAX_COUNT:
            break
        if i % 1000 == 0:
            print(f"OpenThoughts: {count}/{MAX_COUNT}")
        
        try:
            conversations = item['conversations']
            conversation_text = ""
            for turn in conversations:
                role = turn['from'].capitalize()
                if role == 'Gpt':
                    role = 'Assistant'
                conversation_text += "### " + role + ":\n" + turn['value'] + "\n"
            conversation_text += END_TOKEN
            
            encoded = tokenizer.encode(conversation_text)
            all_new_data.append({
                "rawText": conversation_text,
                "tokenizedText": encoded.ids
            })
            count += 1
        except Exception as e:
            print(f"Error processing OpenThoughts sample {i}: {e}")
            continue
    
    print(f"Successfully processed {count} OpenThoughts samples")
except Exception as e:
    print(f"Error loading OpenThoughts dataset: {e}")

# =============================
# 3. Codeforces Python Solutions
# =============================
print("\n=== Processing Codeforces Python Solutions ===")
try:
    ds_code_py = load_dataset("open-r1/codeforces-cots", "solutions_py")
    ds_code_py_train = ds_code_py['train']
    l = len(ds_code_py_train)
    count = 0
    MAX_COUNT = 20_000
    
    for i, item in enumerate(ds_code_py_train):
        if count >= MAX_COUNT:
            break
        if i % 1000 == 0:
            print(f"Codeforces Python: {count}/{MAX_COUNT}")
        
        try:
            messages = item['messages']
            code_text = ""
            for msg in messages:
                if msg['role'] == 'user':
                    code_text += '### User:\n' + msg['content'] + "\n"
                elif msg['role'] == 'assistant':
                    code_text += '### Assistant:\n' + msg['content'] + "\n"
            code_text += END_TOKEN
            
            encoded = tokenizer.encode(code_text)
            all_new_data.append({
                "rawText": code_text,
                "tokenizedText": encoded.ids
            })
            count += 1
        except Exception as e:
            print(f"Error processing Codeforces Python sample {i}: {e}")
            continue
    
    print(f"Successfully processed {count} Codeforces Python samples")
except Exception as e:
    print(f"Error loading Codeforces Python dataset: {e}")

# =============================
# 4. Codeforces General Solutions
# =============================
print("\n=== Processing Codeforces General Solutions ===")
try:
    ds_code = load_dataset("open-r1/codeforces-cots", "solutions")
    ds_code_train = ds_code['train']
    l = len(ds_code_train)
    count = 0
    MAX_COUNT = 20_000
    
    for i, item in enumerate(ds_code_train):
        if count >= MAX_COUNT:
            break
        if i % 1000 == 0:
            print(f"Codeforces General: {count}/{MAX_COUNT}")
        
        try:
            messages = item['messages']
            code_text = ""
            for msg in messages:
                if msg['role'] == 'user':
                    code_text += '### User:\n' + msg['content'] + "\n"
                elif msg['role'] == 'assistant':
                    code_text += '### Assistant:\n' + msg['content'] + "\n"
            code_text += END_TOKEN
            
            encoded = tokenizer.encode(code_text)
            all_new_data.append({
                "rawText": code_text,
                "tokenizedText": encoded.ids
            })
            count += 1
        except Exception as e:
            print(f"Error processing Codeforces General sample {i}: {e}")
            continue
    
    print(f"Successfully processed {count} Codeforces General samples")
except Exception as e:
    print(f"Error loading Codeforces General dataset: {e}")

# =============================
# 5. OpenMathReasoning Dataset
# =============================
print("\n=== Processing OpenMathReasoning Dataset ===")
try:
    ds_maths = load_dataset("nvidia/OpenMathReasoning")
    ds_maths_train = ds_maths[list(ds_maths.keys())[0]]
    l = len(ds_maths_train)
    count = 0
    MAX_COUNT = 20_000
    
    for i, item in enumerate(ds_maths_train):
        if count >= MAX_COUNT:
            break
        if i % 1000 == 0:
            print(f"OpenMathReasoning: {count}/{MAX_COUNT}")
        
        try:
            problem = item['problem']
            solution = item['generated_solution']
            text_content = '### Problem:\n' + problem + "\n" + '### Solution:\n' + solution + " " + END_TOKEN
            
            encoded = tokenizer.encode(text_content)
            all_new_data.append({
                "rawText": text_content,
                "tokenizedText": encoded.ids
            })
            count += 1
        except Exception as e:
            print(f"Error processing Math sample {i}: {e}")
            continue
    
    print(f"Successfully processed {count} Math samples")
except Exception as e:
    print(f"Error loading OpenMathReasoning dataset: {e}")

# =============================
# 6. ArXiv Summarization Dataset
# =============================
print("\n=== Processing ArXiv Summarization Dataset ===")
try:
    ds_summarize = load_dataset("ccdv/arxiv-summarization", "section")
    ds_summarize_train = ds_summarize['train']
    l = len(ds_summarize_train)
    count = 0
    MAX_COUNT = 10_000
    
    for i, item in enumerate(ds_summarize_train):
        if count >= MAX_COUNT:
            break
        if i % 1000 == 0:
            print(f"ArXiv: {count}/{MAX_COUNT}")
        
        try:
            article = item['article']
            abstract = item['abstract']
            
            if len(article) < 50 or len(abstract) < 20:
                continue
            
            text_content = '### Article:\n' + article + "\n" + '### Summary:\n' + abstract + " " + END_TOKEN
            
            encoded = tokenizer.encode(text_content)
            all_new_data.append({
                "rawText": text_content,
                "tokenizedText": encoded.ids
            })
            count += 1
        except Exception as e:
            print(f"Error processing ArXiv sample {i}: {e}")
            continue
    
    print(f"Successfully processed {count} ArXiv samples")
except Exception as e:
    print(f"Error loading ArXiv dataset: {e}")

# =============================
# 7. CSV Files
# =============================
print("\n=== Processing CSV Files ===")
try:
    df = pd.read_csv('train.csv')
    for i, row in enumerate(df.iterrows()):
        if i % 1000 == 0:
            print(f"CSV prompts: {i}/{len(df)}")
        
        try:
            _, row = row
            text = row['prompt'] + "\n" + END_TOKEN
            encoded = tokenizer.encode(text)
            
            all_new_data.append({
                "rawText": text,
                "tokenizedText": encoded.ids
            })
        except Exception as e:
            print(f"Error processing CSV row {i}: {e}")
            continue
    
    print(f"Successfully processed {len(df)} CSV prompts")
except Exception as e:
    print(f"Error loading train.csv: {e}")

try:
    df_python = pd.read_csv('ProblemSolutionPythonV3.csv')
    count = 0
    for i, row in enumerate(df_python.iterrows()):
        if i % 1000 == 0:
            print(f"Python problems: {count}/{len(df_python)}")
        
        try:
            _, row = row
            problem = str(row['Problem']) if pd.notna(row['Problem']) else ''
            python_code = str(row['Python Code']) if pd.notna(row['Python Code']) else ''
            
            if problem and python_code:
                text_content = '### Instruction:\n' + problem + "\n" + '### Output:\n' + python_code + " " + END_TOKEN
                encoded = tokenizer.encode(text_content)
                
                all_new_data.append({
                    "rawText": text_content,
                    "tokenizedText": encoded.ids
                })
                count += 1
        except Exception as e:
            print(f"Error processing Python problem {i}: {e}")
            continue
    
    print(f"Successfully processed {count} Python problems")
except Exception as e:
    print(f"Error loading ProblemSolutionPythonV3.csv: {e}")

# =============================
# 8. Database Data
# =============================
print("\n=== Processing Database Data ===")
try:
    db = sqlite3.connect('../portfolio.db')
    cursor = db.cursor()
    
    try:
        res = cursor.execute('SELECT summary, text FROM news').fetchall()
        for i, row in enumerate(res):
            if i % 1000 == 0:
                print(f"News: {i}/{len(res)}")
            
            try:
                for text in [row[0], row[1]]:
                    text_with_end = text + " " + END_TOKEN
                    encoded = tokenizer.encode(text_with_end)
                    all_new_data.append({
                        "rawText": text_with_end,
                        "tokenizedText": encoded.ids
                    })
            except Exception as e:
                print(f"Error processing news row {i}: {e}")
                continue
        
        print(f"Successfully processed {len(res)*2} news texts")
    except Exception as e:
        print(f"Error querying news: {e}")
    
    try:
        res = cursor.execute('SELECT summary from daily_sentiment').fetchall()
        for i, row in enumerate(res):
            try:
                text = row[0] + " " + END_TOKEN
                encoded = tokenizer.encode(text)
                all_new_data.append({
                    "rawText": text,
                    "tokenizedText": encoded.ids
                })
            except Exception as e:
                print(f"Error processing daily sentiment {i}: {e}")
                continue
        
        print(f"Successfully processed {len(res)} daily sentiment texts")
    except Exception as e:
        print(f"Error querying daily_sentiment: {e}")
    
    try:
        res = cursor.execute('SELECT summary from portfolio_daily_sentiment').fetchall()
        for i, row in enumerate(res):
            try:
                text = row[0] + " " + END_TOKEN
                encoded = tokenizer.encode(text)
                all_new_data.append({
                    "rawText": text,
                    "tokenizedText": encoded.ids
                })
            except Exception as e:
                print(f"Error processing portfolio sentiment {i}: {e}")
                continue
        
        print(f"Successfully processed {len(res)} portfolio sentiment texts")
    except Exception as e:
        print(f"Error querying portfolio_daily_sentiment: {e}")
    
    db.close()
except Exception as e:
    print(f"Error connecting to database: {e}")

# =============================
# 9. Math Samples JSON
# =============================
print("\n=== Processing Math Samples JSON ===")
try:
    with open('math_samples.json', 'r', encoding='utf-8') as f:
        math_samples = json.load(f)
    
    for i, sample in enumerate(math_samples):
        if i % 1000 == 0:
            print(f"Math samples: {i}/{len(math_samples)}")
        
        try:
            prompt = sample.get('prompt', '')
            response = sample.get('response', '')
            difficulty = sample.get('difficulty', 'unknown')
            
            if prompt and response:
                text_content = f"### Math Problem ({difficulty}):\n{prompt}\n### Solution:\n{response} {END_TOKEN}"
                encoded = tokenizer.encode(text_content)
                
                all_new_data.append({
                    "rawText": text_content,
                    "tokenizedText": encoded.ids
                })
        except Exception as e:
            print(f"Error processing math sample {i}: {e}")
            continue
    
    print(f"Successfully processed {len(math_samples)} math samples")
except FileNotFoundError:
    print("math_samples.json not found, skipping...")
except Exception as e:
    print(f"Error loading math_samples.json: {e}")

# =============================
# Merge with Existing File
# =============================
print(f"\n=== Merging {len(all_new_data)} new items ===")

if os.path.exists(output_file):
    print("Merging with existing file...")
    
    try:
        with open(output_file, 'r', encoding='utf-8') as in_f:
            with open(temp_file, 'w', encoding='utf-8') as out_f:
                out_f.write('[\n')
                
                existing = json.load(in_f)
                print(f"Existing items: {len(existing)}")
                
                for i, item in enumerate(existing):
                    json.dump(item, out_f, ensure_ascii=False)
                    out_f.write(',\n')
                
                for i, item in enumerate(all_new_data):
                    json.dump(item, out_f, ensure_ascii=False)
                    if i < len(all_new_data) - 1:
                        out_f.write(',\n')
                    else:
                        out_f.write('\n')
                
                out_f.write(']')
        
        os.replace(temp_file, output_file)
        print(f"Successfully appended {len(all_new_data)} items (total: {len(existing) + len(all_new_data)})")
    except Exception as e:
        print(f"Error during merge: {e}")
        if os.path.exists(temp_file):
            os.remove(temp_file)
else:
    print("Creating new file...")
    try:
        with open(output_file, 'w', encoding='utf-8') as f:
            json.dump(all_new_data, f, indent=4)
        print(f"Created new file with {len(all_new_data)} items")
    except Exception as e:
        print(f"Error creating file: {e}")

print("\n=== Processing Complete ===")