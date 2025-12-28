import sqlite3
import json
import pandas as pd
from tokenizers import Tokenizer
from tokenizers.models import Unigram
from tokenizers.trainers import UnigramTrainer
from tokenizers.pre_tokenizers import Whitespace

df = pd.read_csv('train.csv')
df_python = pd.read_csv('ProblemSolutionPythonV3.csv')

db = sqlite3.connect('../portfolio.db')
cursor = db.cursor()

END_TOKEN = '<EOS>'

texts = []
fullText = ""

res = cursor.execute('SELECT summary, text FROM news').fetchall()
for row in res:
    fullText += row[0] + " " + END_TOKEN + "\n" + row[1] + " " + END_TOKEN + "\n"
    texts.append({'rawText': row[0] + " " + END_TOKEN})
    texts.append({'rawText': row[1] + " " + END_TOKEN})

res = cursor.execute('SELECT summary from daily_sentiment').fetchall()
for row in res:
    fullText += row[0] + " " + END_TOKEN + "\n"
    texts.append({'rawText': row[0] + " " + END_TOKEN})

res = cursor.execute('SELECT summary from portfolio_daily_sentiment').fetchall()
for row in res:
    fullText += row[0] + " " + END_TOKEN + "\n"
    texts.append({'rawText': row[0] + " " + END_TOKEN})

for _, row in df.iterrows():
    fullText += row['prompt'] + "\n" + END_TOKEN
    texts.append({'rawText': row['prompt']})

for _, row in df_python.iterrows():
    problem = str(row['Problem']) if pd.notna(row['Problem']) else ''
    python_code = str(row['Python Code']) if pd.notna(row['Python Code']) else ''
    
    if problem and python_code:
        fullText += '### Instruction:\n' + problem + "\n" + '### Output:' + "\n" + python_code + " " + END_TOKEN + "\n"
        texts.append({'rawText': '### Instruction:\n' + problem + "\n" + '### Output:' + "\n" + python_code + " " + END_TOKEN})

with open('corpus.txt', 'w', encoding='utf-8') as f:
    f.write(fullText)

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
for textObj in texts:
    encoding = tokenizer.encode(textObj['rawText'])
    textObj['tokenizedText'] = encoding.ids

with open('extractedTexts.json', 'w', encoding='utf-8') as f:
    json.dump(texts, f, indent=4)

tokenizer.save('tokenizer.json')

max_len = max(len(textObj['tokenizedText']) for textObj in texts)
print(f'Maximum tokenized text length: {max_len}')
print(f"Vocabulary size: {tokenizer.get_vocab_size()}")
print(f"Number of texts: {len(texts)}")


