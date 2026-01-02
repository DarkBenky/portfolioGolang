import json
import os
import sys

def optimize_dataset(input_path, output_path=None):
    if output_path is None:
        base, ext = os.path.splitext(input_path)
        output_path = f"{base}.tokens.json"
    
    if os.path.exists(output_path):
        print(f"Optimized file already exists: {output_path}")
        response = input("Overwrite? (y/n): ").lower()
        if response != 'y':
            print("Skipping...")
            return False
    
    print(f"Loading: {input_path}")
    file_size_mb = os.path.getsize(input_path) / (1024 * 1024)
    print(f"File size: {file_size_mb:.2f} MB")
    
    with open(input_path, 'r', encoding='utf-8') as f:
        data = json.load(f)
    
    print(f"Total entries: {len(data)}")
    
    tokenized_arrays = []
    for item in data:
        if 'tokenizedText' in item and len(item['tokenizedText']) > 1:
            tokenized_arrays.append(item['tokenizedText'])
    
    print(f"Valid tokenized sequences: {len(tokenized_arrays)}")
    
    print(f"Saving optimized data to: {output_path}")
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(tokenized_arrays, f)
    
    optimized_size_mb = os.path.getsize(output_path) / (1024 * 1024)
    print(f"Optimized file size: {optimized_size_mb:.2f} MB")
    print(f"Size reduction: {((file_size_mb - optimized_size_mb) / file_size_mb * 100):.1f}%")
    print(f"Completed: {output_path}\n")
    
    return True

def main():
    if len(sys.argv) < 2:
        print("Usage: python optimizeDatasets.py <json_file1> [json_file2] ...")
        print("\nExample:")
        print("  python optimizeDatasets.py extractedTexts.json")
        print("  python optimizeDatasets.py extractedTexts.json extractedTexts_multilang.json")
        print("\nThis will create optimized .tokens.json files containing only token indices.")
        sys.exit(1)
    
    input_files = sys.argv[1:]
    
    print(f"Processing {len(input_files)} file(s)...\n")
    
    successful = 0
    for input_path in input_files:
        if not os.path.exists(input_path):
            print(f"Error: File not found: {input_path}\n")
            continue
        
        if optimize_dataset(input_path):
            successful += 1
    
    print(f"\nCompleted: {successful}/{len(input_files)} files optimized")

if __name__ == "__main__":
    main()
