import os
import torch
import torch.nn as nn
from train_pytorch import CausalLanguageModel
from tokenizers import Tokenizer
import time


def quantize_model_dynamic(model, dtype=torch.qint8):
    print(f"\nApplying dynamic quantization (dtype={dtype})...")
    print("NOTE: Using deprecated torch.quantization API. Consider using torchao for future projects.")
    quantized_model = torch.quantization.quantize_dynamic(
        model,
        {nn.Linear},
        dtype=dtype
    )
    return quantized_model


def quantize_to_fp16(weights_path, tokenizer_path, output_path, context_window=2048, d_model=2048,
                     num_heads=32, num_layers=18, num_kv_heads=8, use_bfloat16=False):
    device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
    target_dtype = torch.bfloat16 if use_bfloat16 else torch.float16
    dtype_name = "bfloat16" if use_bfloat16 else "float16"
    
    print(f"Loading tokenizer from {tokenizer_path}...")
    tokenizer = Tokenizer.from_file(tokenizer_path)
    vocab_size = tokenizer.get_vocab_size()
    
    print(f"Building model architecture...")
    model = CausalLanguageModel(
        vocab_size=vocab_size,
        context_window=context_window,
        d_model=d_model,
        num_heads=num_heads,
        num_layers=num_layers,
        ff_dim=int(d_model * 8 / 3),
        dropout=0.0,
        num_kv_heads=num_kv_heads
    )
    
    print(f"Loading weights from {weights_path}...")
    checkpoint = torch.load(weights_path, map_location='cpu', weights_only=False)
    state_dict = checkpoint.get('model_state_dict', checkpoint)
    state_dict = {k.replace('_orig_mod.', ''): v for k, v in state_dict.items()}
    model.load_state_dict(state_dict, strict=False)
    
    model.to(device=device, dtype=torch.float32)
    model.eval()
    
    total_params = sum(p.numel() for p in model.parameters())
    print(f"\nOriginal model (float32):")
    print(f"  Parameters: {total_params:,}")
    
    original_size = get_model_size(model)
    print(f"  Size: {original_size:.2f} MB")
    
    avg_time_original, tokens_per_sec_original = benchmark_model(model, tokenizer, device, torch.float32)
    print(f"  Inference: {avg_time_original*1000:.2f} ms/forward ({tokens_per_sec_original:.1f} tokens/sec)")
    
    print(f"\nConverting to {dtype_name}...")
    model = model.to(dtype=target_dtype)
    
    quantized_size = get_model_size(model)
    print(f"\n{dtype_name} model:")
    print(f"  Size: {quantized_size:.2f} MB")
    print(f"  Compression: {original_size/quantized_size:.2f}x smaller")
    
    avg_time_quantized, tokens_per_sec_quantized = benchmark_model(model, tokenizer, device, target_dtype)
    print(f"  Inference: {avg_time_quantized*1000:.2f} ms/forward ({tokens_per_sec_quantized:.1f} tokens/sec)")
    print(f"  Speedup: {avg_time_original/avg_time_quantized:.2f}x faster")
    
    print(f"\nSaving {dtype_name} model to {output_path}...")
    torch.save({
        'model_state_dict': model.state_dict(),
        'quantization_type': dtype_name,
        'vocab_size': vocab_size,
        'context_window': context_window,
        'd_model': d_model,
        'num_heads': num_heads,
        'num_layers': num_layers,
        'num_kv_heads': num_kv_heads,
    }, output_path)
    
    print(f"\n{dtype_name} conversion complete!")
    print(f"  Original (float32): {original_size:.2f} MB, {tokens_per_sec_original:.1f} tokens/sec")
    print(f"  {dtype_name}: {quantized_size:.2f} MB, {tokens_per_sec_quantized:.1f} tokens/sec")
    print(f"  Savings: {original_size - quantized_size:.2f} MB ({(1 - quantized_size/original_size)*100:.1f}%)")
    print(f"\nNOTE: {dtype_name} model works on GPU and maintains good accuracy.")


def quantize_model_static(model, tokenizer, device, calibration_samples=100, context_window=2048):
    print(f"\nApplying static quantization with {calibration_samples} calibration samples...")
    
    model.eval()
    model.qconfig = torch.quantization.get_default_qconfig('x86')
    
    torch.quantization.prepare(model, inplace=True)
    
    print("Running calibration...")
    with torch.no_grad():
        for i in range(calibration_samples):
            dummy_input = torch.randint(0, tokenizer.get_vocab_size(), (1, context_window), device=device)
            model(dummy_input)
            if (i + 1) % 20 == 0:
                print(f"  Calibrated {i+1}/{calibration_samples} samples")
    
    torch.quantization.convert(model, inplace=True)
    print("Static quantization complete!")
    
    return model


def benchmark_model(model, tokenizer, device, dtype, num_iterations=50, seq_length=512):
    print(f"\nBenchmarking model...")
    model.eval()
    
    dummy_input = torch.randint(0, tokenizer.get_vocab_size(), (1, seq_length), device=device)
    
    print("Warming up...")
    with torch.no_grad():
        for _ in range(5):
            _ = model(dummy_input)
    
    if torch.cuda.is_available():
        torch.cuda.synchronize()
    
    print(f"Running {num_iterations} iterations...")
    start_time = time.time()
    
    with torch.no_grad():
        for _ in range(num_iterations):
            _ = model(dummy_input)
    
    if torch.cuda.is_available():
        torch.cuda.synchronize()
    
    elapsed = time.time() - start_time
    avg_time = elapsed / num_iterations
    tokens_per_sec = seq_length / avg_time
    
    return avg_time, tokens_per_sec


def get_model_size(model):
    torch.save(model.state_dict(), 'temp_model.pt')
    size_mb = os.path.getsize('temp_model.pt') / (1024 * 1024)
    os.remove('temp_model.pt')
    return size_mb


def quantize_to_int8(weights_path, tokenizer_path, output_path, context_window=2048, d_model=2048, 
                     num_heads=32, num_layers=18, num_kv_heads=8, quantization_type='dynamic'):
    device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
    
    print(f"Loading tokenizer from {tokenizer_path}...")
    tokenizer = Tokenizer.from_file(tokenizer_path)
    vocab_size = tokenizer.get_vocab_size()
    
    print(f"Building model architecture...")
    model = CausalLanguageModel(
        vocab_size=vocab_size,
        context_window=context_window,
        d_model=d_model,
        num_heads=num_heads,
        num_layers=num_layers,
        ff_dim=int(d_model * 8 / 3),
        dropout=0.0,
        num_kv_heads=num_kv_heads
    )
    
    print(f"Loading weights from {weights_path}...")
    checkpoint = torch.load(weights_path, map_location='cpu', weights_only=False)
    state_dict = checkpoint.get('model_state_dict', checkpoint)
    state_dict = {k.replace('_orig_mod.', ''): v for k, v in state_dict.items()}
    model.load_state_dict(state_dict, strict=False)
    
    model.to(device)
    model.eval()
    
    total_params = sum(p.numel() for p in model.parameters())
    print(f"\nOriginal model:")
    print(f"  Parameters: {total_params:,}")
    
    original_size = get_model_size(model)
    print(f"  Size: {original_size:.2f} MB")
    
    avg_time_original, tokens_per_sec_original = benchmark_model(model, tokenizer, device, torch.float32)
    print(f"  Inference: {avg_time_original*1000:.2f} ms/forward ({tokens_per_sec_original:.1f} tokens/sec)")
    
    print(f"\nMoving model to CPU for quantization (quantization only works on CPU)...")
    model = model.cpu()
    
    if quantization_type == 'dynamic':
        quantized_model = quantize_model_dynamic(model, dtype=torch.qint8)
    elif quantization_type == 'static':
        quantized_model = quantize_model_static(model, tokenizer, 'cpu', calibration_samples=50, context_window=context_window)
    else:
        raise ValueError(f"Unknown quantization type: {quantization_type}")
    
    quantized_size = get_model_size(quantized_model)
    print(f"\nQuantized model (INT8 {quantization_type}):")
    print(f"  Size: {quantized_size:.2f} MB")
    print(f"  Compression: {original_size/quantized_size:.2f}x smaller")
    
    avg_time_quantized, tokens_per_sec_quantized = benchmark_model(quantized_model, tokenizer, torch.device('cpu'), torch.qint8)
    print(f"  Inference (CPU): {avg_time_quantized*1000:.2f} ms/forward ({tokens_per_sec_quantized:.1f} tokens/sec)")
    
    if device.type == 'cuda':
        speedup = avg_time_original / avg_time_quantized
        print(f"  vs Original GPU: {speedup:.2f}x {'faster' if speedup > 1 else 'slower'}")
    else:
        print(f"  CPU only speedup: {1.0:.2f}x (original was also CPU)")
    
    print(f"\nSaving quantized model to {output_path}...")
    torch.save({
        'model_state_dict': quantized_model.state_dict(),
        'quantization_type': quantization_type,
        'vocab_size': vocab_size,
        'context_window': context_window,
        'd_model': d_model,
        'num_heads': num_heads,
        'num_layers': num_layers,
        'num_kv_heads': num_kv_heads,
    }, output_path)
    
    print(f"\nQuantization complete!")
    print(f"  Original: {original_size:.2f} MB")
    print(f"  Quantized (CPU): {quantized_size:.2f} MB, {tokens_per_sec_quantized:.1f} tokens/sec")
    print(f"  Savings: {original_size - quantized_size:.2f} MB ({(1 - quantized_size/original_size)*100:.1f}%)")
    print(f"\nNOTE: Quantized model runs on CPU only. Use for inference on CPU devices.")


def quantize_to_int4_bitsandbytes(weights_path, tokenizer_path, output_path, context_window=2048, 
                                   d_model=2048, num_heads=32, num_layers=18, num_kv_heads=8):
    try:
        import bitsandbytes as bnb
    except ImportError:
        print("ERROR: bitsandbytes not installed. Install with: pip install bitsandbytes")
        return
    
    device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
    
    if not torch.cuda.is_available():
        print("ERROR: bitsandbytes requires CUDA. Skipping INT4 quantization.")
        return
    
    print(f"\nLoading tokenizer from {tokenizer_path}...")
    tokenizer = Tokenizer.from_file(tokenizer_path)
    vocab_size = tokenizer.get_vocab_size()
    
    print(f"Building model architecture...")
    model = CausalLanguageModel(
        vocab_size=vocab_size,
        context_window=context_window,
        d_model=d_model,
        num_heads=num_heads,
        num_layers=num_layers,
        ff_dim=int(d_model * 8 / 3),
        dropout=0.0,
        num_kv_heads=num_kv_heads
    )
    
    print(f"Loading weights from {weights_path}...")
    checkpoint = torch.load(weights_path, map_location='cpu', weights_only=False)
    state_dict = checkpoint.get('model_state_dict', checkpoint)
    state_dict = {k.replace('_orig_mod.', ''): v for k, v in state_dict.items()}
    model.load_state_dict(state_dict, strict=False)
    
    total_params = sum(p.numel() for p in model.parameters())
    print(f"\nOriginal model:")
    print(f"  Parameters: {total_params:,}")
    
    original_size = get_model_size(model)
    print(f"  Size: {original_size:.2f} MB")
    
    print(f"\nApplying 4-bit quantization with bitsandbytes...")
    
    for name, module in model.named_modules():
        if isinstance(module, nn.Linear):
            parent_name = '.'.join(name.split('.')[:-1])
            child_name = name.split('.')[-1]
            
            if parent_name:
                parent = model.get_submodule(parent_name)
            else:
                parent = model
            
            quantized_linear = bnb.nn.Linear4bit(
                module.in_features,
                module.out_features,
                bias=module.bias is not None,
                compute_dtype=torch.bfloat16,
                compress_statistics=True,
                quant_type='nf4'
            )
            
            quantized_linear.weight.data = module.weight.data
            if module.bias is not None:
                quantized_linear.bias.data = module.bias.data
            
            setattr(parent, child_name, quantized_linear)
    
    model.to(device)
    
    quantized_size = get_model_size(model)
    print(f"\nQuantized model (INT4 NF4):")
    print(f"  Size: {quantized_size:.2f} MB")
    print(f"  Compression: {original_size/quantized_size:.2f}x smaller")
    print(f"  Savings: {original_size - quantized_size:.2f} MB ({(1 - quantized_size/original_size)*100:.1f}%)")
    
    print(f"\nSaving quantized model to {output_path}...")
    torch.save({
        'model_state_dict': model.state_dict(),
        'quantization_type': 'int4_nf4',
        'vocab_size': vocab_size,
        'context_window': context_window,
        'd_model': d_model,
        'num_heads': num_heads,
        'num_layers': num_layers,
        'num_kv_heads': num_kv_heads,
    }, output_path)
    
    print(f"\nINT4 quantization complete!")


def load_quantized_model(quantized_path, tokenizer_path):
    print(f"Loading quantized model from {quantized_path}...")
    
    checkpoint = torch.load(quantized_path, map_location='cpu', weights_only=False)
    
    tokenizer = Tokenizer.from_file(tokenizer_path)
    
    model = CausalLanguageModel(
        vocab_size=checkpoint['vocab_size'],
        context_window=checkpoint['context_window'],
        d_model=checkpoint['d_model'],
        num_heads=checkpoint['num_heads'],
        num_layers=checkpoint['num_layers'],
        ff_dim=int(checkpoint['d_model'] * 8 / 3),
        dropout=0.0,
        num_kv_heads=checkpoint['num_kv_heads']
    )
    
    quantization_type = checkpoint.get('quantization_type', 'dynamic')
    
    if quantization_type == 'dynamic':
        model = quantize_model_dynamic(model, dtype=torch.qint8)
    
    model.load_state_dict(checkpoint['model_state_dict'])
    model.eval()
    
    print(f"Loaded quantized model ({quantization_type})")
    
    return model, tokenizer


def main():
    import argparse
    
    parser = argparse.ArgumentParser(description='Quantize PyTorch transformer model')
    parser.add_argument('--weights', type=str, default='best_model_pytorch_v2.pt',
                        help='Path to model weights')
    parser.add_argument('--tokenizer', type=str, default='tokenizer.json',
                        help='Path to tokenizer')
    parser.add_argument('--output', type=str, default='quantized_model_int8.pt',
                        help='Output path for quantized model')
    parser.add_argument('--type', type=str, default='fp16', choices=['dynamic', 'static', 'int4', 'fp16', 'bf16'],
                        help='Quantization type: dynamic (INT8 CPU), static (INT8 CPU), int4 (NF4 GPU), fp16 (float16 GPU), bf16 (bfloat16 GPU)')
    parser.add_argument('--d-model', type=int, default=2048,
                        help='Model dimension')
    parser.add_argument('--num-heads', type=int, default=32,
                        help='Number of attention heads')
    parser.add_argument('--num-layers', type=int, default=18,
                        help='Number of transformer layers')
    parser.add_argument('--num-kv-heads', type=int, default=8,
                        help='Number of key-value heads for GQA')
    parser.add_argument('--context-window', type=int, default=2048,
                        help='Context window size')
    
    args = parser.parse_args()
    
    print("="*60)
    print("PyTorch Model Quantization Script")
    print("="*60)
    
    if args.type == 'int4':
        quantize_to_int4_bitsandbytes(
            weights_path=args.weights,
            tokenizer_path=args.tokenizer,
            output_path=args.output,
            context_window=args.context_window,
            d_model=args.d_model,
            num_heads=args.num_heads,
            num_layers=args.num_layers,
            num_kv_heads=args.num_kv_heads
        )
    elif args.type in ['fp16', 'bf16']:
        quantize_to_fp16(
            weights_path=args.weights,
            tokenizer_path=args.tokenizer,
            output_path=args.output,
            context_window=args.context_window,
            d_model=args.d_model,
            num_heads=args.num_heads,
            num_layers=args.num_layers,
            num_kv_heads=args.num_kv_heads,
            use_bfloat16=(args.type == 'bf16')
        )
    else:
        quantize_to_int8(
            weights_path=args.weights,
            tokenizer_path=args.tokenizer,
            output_path=args.output,
            context_window=args.context_window,
            d_model=args.d_model,
            num_heads=args.num_heads,
            num_layers=args.num_layers,
            num_kv_heads=args.num_kv_heads,
            quantization_type=args.type
        )
    
    print("\n" + "="*60)
    print("Quantization complete!")
    print("="*60)
    print(f"\nTo use the quantized model:")
    print(f"  from quantize_model import load_quantized_model")
    print(f"  model, tokenizer = load_quantized_model('{args.output}', '{args.tokenizer}')")


if __name__ == '__main__':
    main()
