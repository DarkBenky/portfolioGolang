import sqlite3
import json
from tensorflow.keras.preprocessing.text import Tokenizer
import pandas as pd

df = pd.read_csv('train.csv')

db = sqlite3.connect('../portfolio.db')
cursor = db.cursor()

texts = []
fullText = ""

res = cursor.execute('SELECT  summary, text FROM news').fetchall()
for row in res:
    fullText += row[0] + "\n" + row[1] + "\n"
    texts.append({
        'rawText': row[0],
        'tokenizedText': 'N/A'
    })
    texts.append({
        'rawText': row[1],
        'tokenizedText': 'N/A'
    })

res = cursor.execute('SELECT summary from daily_sentiment').fetchall()
for row in res:
    fullText += row[0] + "\n"
    texts.append({
        'rawText': row[0],
        'tokenizedText': 'N/A'
    })

res = cursor.execute('SELECT summary from portfolio_daily_sentiment').fetchall()
for row in res:
    fullText += row[0] + "\n"
    texts.append({
        'rawText': row[0],
        'tokenizedText': 'N/A'
    })

for _, row in df.iterrows():
    fullText += row['prompt'] + "\n"
    texts.append({
        'rawText': row['prompt'],
        'tokenizedText': 'N/A'
    })

tokenizer = Tokenizer()
tokenizer.fit_on_texts([fullText])

for textObj in texts:
    tokens = tokenizer.texts_to_sequences([textObj['rawText']])
    textObj['tokenizedText'] = tokens[0]

with open('extractedTexts.json', 'w') as f:
    json.dump(texts, f, indent=4)

# Save the tokenizer for future use
with open('tokenizer.json', 'w') as f:
    json.dump(tokenizer.to_json(), f, indent=4)

max_len = max(len(textObj['tokenizedText']) for textObj in texts)
print(f'Maximum tokenized text length: {max_len}')
print(f"Vocabulary size: {len(tokenizer.word_index) + 1}")
