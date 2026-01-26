import os
import torch
import torch.nn as nn
import torch.nn.functional as F
from torch.utils.data import IterableDataset, DataLoader
import wandb
from dataGen import DataGenerator, GoDataGenerator
import math
import time

try:
    from flash_attn import flash_attn_qkvpacked_func, flash_attn_func
    FLASH_AVAILABLE = True
except ImportError:
    FLASH_AVAILABLE = False
    print("FlashAttention not available, using standard attention")


class RMSNorm(nn.Module):
    def __init__(self, dim, eps=1e-6):
        super().__init__()
        self.eps = eps
        self.weight = nn.Parameter(torch.ones(dim))

    def forward(self, x):
        rms = torch.sqrt(torch.mean(x ** 2, dim=-1, keepdim=True) + self.eps)
        x_normed = x / rms
        return self.weight * x_normed


class SwiGLU(nn.Module):
    def __init__(self, d_model, hidden_dim, dropout=0.0, bias=False):
        super().__init__()
        self.w1 = nn.Linear(d_model, hidden_dim, bias=bias)
        self.w2 = nn.Linear(d_model, hidden_dim, bias=bias)
        self.w3 = nn.Linear(hidden_dim, d_model, bias=bias)
        self.dropout = nn.Dropout(dropout)

    def forward(self, x):
        return self.w3(self.dropout(F.silu(self.w1(x)) * self.w2(x)))


class RotaryEmbedding(nn.Module):
    def __init__(self, dim, max_seq_len=8192, base=10000):
        super().__init__()
        inv_freq = 1.0 / (base ** (torch.arange(0, dim, 2).float() / dim))
        self.register_buffer("inv_freq", inv_freq)
        self.max_seq_len = max_seq_len
        self.seq_len_cached = None
        self.cos_cached = None
        self.sin_cached = None

    def forward(self, x, seq_len=None):
        if seq_len is None:
            seq_len = x.shape[1]
        
        if seq_len != self.seq_len_cached or (self.cos_cached is not None and self.cos_cached.dtype != x.dtype):
            self.seq_len_cached = seq_len
            t = torch.arange(seq_len, device=x.device, dtype=x.dtype)
            inv_freq = self.inv_freq.to(dtype=x.dtype, device=x.device)
            freqs = torch.einsum("i,j->ij", t, inv_freq)
            emb = torch.cat((freqs, freqs), dim=-1)
            self.cos_cached = emb.cos()[None, :, None, :]
            self.sin_cached = emb.sin()[None, :, None, :]
        
        return self.cos_cached, self.sin_cached


def apply_rotary_pos_emb(q, k, cos, sin):
    def rotate_half(x):
        x1, x2 = x[..., : x.shape[-1] // 2], x[..., x.shape[-1] // 2 :]
        return torch.cat((-x2, x1), dim=-1)
    
    q_embed = (q * cos) + (rotate_half(q) * sin)
    k_embed = (k * cos) + (rotate_half(k) * sin)
    return q_embed, k_embed


class FlashAttentionBlock(nn.Module):
    def __init__(self, d_model, num_heads, dropout=0.0, bias=False, num_kv_heads=None):
        super().__init__()
        assert d_model % num_heads == 0
        self.d_model = d_model
        self.num_heads = num_heads
        self.num_kv_heads = num_kv_heads if num_kv_heads is not None else num_heads
        assert num_heads % self.num_kv_heads == 0
        self.num_queries_per_kv = num_heads // self.num_kv_heads
        
        self.head_dim = d_model // num_heads
        
        self.q_proj = nn.Linear(d_model, num_heads * self.head_dim, bias=bias)
        self.k_proj = nn.Linear(d_model, self.num_kv_heads * self.head_dim, bias=bias)
        self.v_proj = nn.Linear(d_model, self.num_kv_heads * self.head_dim, bias=bias)
        self.proj = nn.Linear(d_model, d_model, bias=bias)
        self.dropout = dropout
        self.rotary = RotaryEmbedding(self.head_dim)

    def forward(self, x):
        batch_size, seq_len, _ = x.shape
        
        q = self.q_proj(x)
        k = self.k_proj(x)
        v = self.v_proj(x)
        
        q = q.view(batch_size, seq_len, self.num_heads, self.head_dim)
        k = k.view(batch_size, seq_len, self.num_kv_heads, self.head_dim)
        v = v.view(batch_size, seq_len, self.num_kv_heads, self.head_dim)
        
        cos, sin = self.rotary(x, seq_len)
        q, k = apply_rotary_pos_emb(q, k, cos, sin)
        
        if FLASH_AVAILABLE and x.dtype in [torch.float16, torch.bfloat16]:
            out = flash_attn_func(
                q, k, v,
                dropout_p=self.dropout if self.training else 0.0,
                causal=True
            )
            out = out.reshape(batch_size, seq_len, self.d_model)
        else:
            if self.num_kv_heads != self.num_heads:
                k = k.repeat_interleave(self.num_queries_per_kv, dim=2)
                v = v.repeat_interleave(self.num_queries_per_kv, dim=2)
            
            q = q.transpose(1, 2)
            k = k.transpose(1, 2)
            v = v.transpose(1, 2)
            
            from torch.nn.attention import SDPBackend, sdpa_kernel
            with sdpa_kernel([SDPBackend.MATH, SDPBackend.EFFICIENT_ATTENTION]):
                out = F.scaled_dot_product_attention(
                    q, k, v,
                    attn_mask=None,
                    dropout_p=self.dropout if self.training else 0.0,
                    is_causal=True
                )
            
            out = out.transpose(1, 2)
            out = out.reshape(batch_size, seq_len, self.d_model)
        
        return self.proj(out)


class TransformerBlock(nn.Module):
    def __init__(self, d_model, num_heads, ff_dim, dropout=0.0, layer_idx=0, num_layers=1, num_kv_heads=None):
        super().__init__()
        self.attention = FlashAttentionBlock(d_model, num_heads, dropout, num_kv_heads=num_kv_heads)
        self.feed_forward = SwiGLU(d_model, ff_dim, dropout)
        self.norm1 = RMSNorm(d_model)
        self.norm2 = RMSNorm(d_model)
        self.dropout = nn.Dropout(dropout)

    def forward(self, x):
        residual = x
        x = self.norm1(x)
        x = residual + self.dropout(self.attention(x))
        
        residual = x
        x = self.norm2(x)
        x = residual + self.dropout(self.feed_forward(x))
        
        return x


class CausalLanguageModel(nn.Module):
    def __init__(self, vocab_size, context_window, d_model, num_heads, num_layers, ff_dim, dropout=0.0, num_kv_heads=None):
        super().__init__()
        self.vocab_size = vocab_size
        self.d_model = d_model
        self._gradient_checkpointing = False
        
        self.token_embedding = nn.Embedding(vocab_size, d_model)
        self.dropout = nn.Dropout(dropout)
        
        self.blocks = nn.ModuleList([
            TransformerBlock(d_model, num_heads, ff_dim, dropout, i, num_layers, num_kv_heads)
            for i in range(num_layers)
        ])
        
        self.final_norm = RMSNorm(d_model)
        
        self._init_weights()

    def _init_weights(self):
        for module in self.modules():
            if isinstance(module, nn.Linear):
                torch.nn.init.normal_(module.weight, mean=0.0, std=0.02)
                if module.bias is not None:
                    torch.nn.init.zeros_(module.bias)
            elif isinstance(module, nn.Embedding):
                torch.nn.init.normal_(module.weight, mean=0.0, std=0.02)
    
    def add_layers(self, num_new_layers, num_heads, ff_dim, dropout=0.0, num_kv_heads=None):
        current_layers = len(self.blocks)
        new_blocks = []
        device = next(self.parameters()).device
        dtype = next(self.parameters()).dtype
        
        for i in range(num_new_layers):
            block = TransformerBlock(
                self.d_model, num_heads, ff_dim, dropout, 
                current_layers + i, current_layers + num_new_layers, num_kv_heads
            )
            
            for module in block.modules():
                if isinstance(module, nn.Linear):
                    if 'proj' in str(type(module).__name__).lower() or hasattr(module, 'out_features'):
                        torch.nn.init.zeros_(module.weight)
                        if module.bias is not None:
                            torch.nn.init.zeros_(module.bias)
            
            block = block.to(device=device, dtype=dtype)
            new_blocks.append(block)
        
        self.blocks.extend(new_blocks)
        print(f"Added {num_new_layers} new layers (initialized as passthrough)")
        print(f"Total layers now: {len(self.blocks)}")

    def forward(self, x):
        x = self.token_embedding(x)
        x = self.dropout(x)
        
        for block in self.blocks:
            if self._gradient_checkpointing and self.training:
                x = torch.utils.checkpoint.checkpoint(block, x, use_reentrant=False)
            else:
                x = block(x)
        
        x = self.final_norm(x)
        
        logits = F.linear(x, self.token_embedding.weight)
        
        return logits

    def count_parameters(self):
        return sum(p.numel() for p in self.parameters() if p.requires_grad)
    
    def gradient_checkpointing_enable(self):
        self._gradient_checkpointing = True
        
    def gradient_checkpointing_disable(self):
        self._gradient_checkpointing = False


class TokenDataset(IterableDataset):
    def __init__(self, data_generator, batch_size, context_window):
        self.data_generator = data_generator
        self.batch_size = batch_size
        self.context_window = context_window

    def __iter__(self):
        for batch_data in self.data_generator.generate_batches(
            self.batch_size, self.context_window
        ):
            if len(batch_data) == 4:
                X_batch, Y_batch, mask_batch, total_tokens = batch_data
            else:
                X_batch, Y_batch, mask_batch = batch_data
                total_tokens = int(mask_batch.sum())
            
            X_batch = torch.from_numpy(X_batch).long()
            Y_batch = torch.from_numpy(Y_batch).long()
            mask_batch = torch.from_numpy(mask_batch).float()
            yield X_batch, Y_batch, mask_batch, total_tokens


def get_cosine_schedule_with_warmup(optimizer, num_warmup_steps, num_training_steps, min_lr_ratio=0.1):
    def lr_lambda(current_step):
        if current_step < num_warmup_steps:
            return float(current_step) / float(max(1, num_warmup_steps))
        progress = float(current_step - num_warmup_steps) / float(max(1, num_training_steps - num_warmup_steps))
        return max(min_lr_ratio, 0.5 * (1.0 + math.cos(math.pi * progress)))
    
    return torch.optim.lr_scheduler.LambdaLR(optimizer, lr_lambda)


def get_exp_decay_schedule(optimizer, num_warmup_steps, decay_steps, decay_rate=0.96, min_lr_ratio=0.1):
    def lr_lambda(current_step):
        if current_step < num_warmup_steps:
            return float(current_step) / float(max(1, num_warmup_steps))
        decay_progress = (current_step - num_warmup_steps) // decay_steps
        return max(min_lr_ratio, decay_rate ** decay_progress)
    
    return torch.optim.lr_scheduler.LambdaLR(optimizer, lr_lambda)


def train():
    CONTEXT_WINDOW = 2048
    D_MODEL = 2048
    NUM_HEADS = 32
    NUM_KV_HEADS = 8
    NUM_LAYERS = 21
    ADD_NEW_LAYERS = 0
    BATCH_SIZE = 2
    LEARNING_RATE = 0.0002
    WEIGHT_DECAY = 0.1
    WARMUP_STEPS = 0
    EPOCHS = 10000
    FFN_DIM = int(D_MODEL * 8 / 3)
    DROPOUT_RATE = 0.0
    WEIGHTS_PATH = 'best_model_pytorch_v2.pt'
    LOAD_BEST_MODEL = True
    MAX_GRAD_NORM = 1.0
    USE_GO_SERVER = True
    GO_SERVER_URL = "http://localhost:4567"
    USE_GRADIENT_CHECKPOINTING = True
    USE_TORCH_COMPILE = False
    COMPILE_MODE = "default"
    
    device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
    dtype = torch.bfloat16
    
    torch.set_default_dtype(torch.bfloat16)
    
    print(f"Using device: {device}")
    print(f"Using dtype: {dtype}")
    print(f"FlashAttention available: {FLASH_AVAILABLE}")
    print(f"Using Go server for data: {USE_GO_SERVER}")
    
    wandb_run_id = "akgunoxc" if ADD_NEW_LAYERS == 0 else None
    if LOAD_BEST_MODEL and os.path.exists(WEIGHTS_PATH):
        try:
            checkpoint = torch.load(WEIGHTS_PATH, map_location='cpu', weights_only=False)
            saved_run_id = checkpoint.get('wandb_run_id', None)
            if saved_run_id and ADD_NEW_LAYERS == 0:
                wandb_run_id = saved_run_id
                print(f"Resuming wandb run from checkpoint: {wandb_run_id}")
            else:
                if ADD_NEW_LAYERS > 0:
                    print(f"Starting NEW wandb run (adding {ADD_NEW_LAYERS} layers)")
                else:
                    print(f"Using hardcoded wandb run: {wandb_run_id}")
        except Exception as e:
            print(f"Could not load wandb_run_id from checkpoint: {e}")
            if ADD_NEW_LAYERS == 0:
                print(f"Using hardcoded wandb run: {wandb_run_id}")
    
    wandb.init(
        project="portfolio-transformer-v2",
        id=wandb_run_id,
        resume="allow" if wandb_run_id else None,
        config={
            "context_window": CONTEXT_WINDOW,
            "add_new_layers": ADD_NEW_LAYERS,
            "total_layers": NUM_LAYERS + ADD_NEW_LAYERS,
            "batch_size": BATCH_SIZE,
            "learning_rate": LEARNING_RATE,
            "weight_decay": WEIGHT_DECAY,
            "warmup_steps": WARMUP_STEPS,
            "epochs": EPOCHS,
            "ffn_dim": FFN_DIM,
            "dropout_rate": DROPOUT_RATE,
            "dtype": str(dtype),
            "flash_attention": FLASH_AVAILABLE,
            "improvements": "GQA + RMSNorm + SwiGLU + RoPE + FlashAttention2 + WeightTying + GradAccum + GoDataServer + GradCheckpoint + TorchCompile" + (" + ProgressiveLayerAdding" if ADD_NEW_LAYERS > 0 else ""),
            "load_best_model": LOAD_BEST_MODEL,
            "use_go_server": USE_GO_SERVER,
            "use_gradient_checkpointing": USE_GRADIENT_CHECKPOINTING,
            "use_torch_compile": USE_TORCH_COMPILE,
            "compile_mode": COMPILE_MODE if USE_TORCH_COMPILE else None
        }
    )
    
    if USE_GO_SERVER:
        from tokenizers import Tokenizer
        tokenizer = Tokenizer.from_file('tokenizer.json')
        vocab_size = tokenizer.get_vocab_size()
        data_gen = GoDataGenerator(vocab_size=vocab_size, go_server_url=GO_SERVER_URL)
    else:
        data_gen = DataGenerator(
            [
                '/media/user/2TB/extractedTexts_multilang.json',
                '/media/user/2TB/extractedTexts_CodeMix.json',
                '/media/user/2TB/extractedTexts.json',
                '/media/user/2TB/extractedTexts_OpenThoughts.json',
                '/media/user/2TB/extractedTexts_FineBert.json',
                '/media/user/2TB/extractedTexts_Math.json'
            ],
            'tokenizer.json',
            lazy_count=True
        )
        data_gen.preload_all_files()
        vocab_size = data_gen.vocab_size
    
    wandb.config.update({"vocab_size": vocab_size})
    
    model = CausalLanguageModel(
        vocab_size=vocab_size,
        context_window=CONTEXT_WINDOW,
        d_model=D_MODEL,
        num_heads=NUM_HEADS,
        num_layers=NUM_LAYERS,
        ff_dim=FFN_DIM,
        dropout=DROPOUT_RATE,
        num_kv_heads=NUM_KV_HEADS
    )
    
    model = model.to(device=device, dtype=dtype)
    
    if USE_GRADIENT_CHECKPOINTING:
        model.gradient_checkpointing_enable()
        print("Gradient checkpointing enabled")
    
    total_params = model.count_parameters()
    print(f"\nTotal parameters: {total_params:,}")
    print(f"Model size: ~{total_params * 2 / 1e9:.2f} GB (bfloat16)")
    
    start_epoch = 0
    best_loss = None
    total_samples_trained = 0
    total_tokens_trained = 0
    
    if LOAD_BEST_MODEL and os.path.exists(WEIGHTS_PATH):
        try:
            print(f"\n{'='*60}")
            print(f"LOADING MODEL from {WEIGHTS_PATH}")
            print(f"{'='*60}\n")
            
            checkpoint = torch.load(WEIGHTS_PATH, map_location=device, weights_only=False)
            
            state_dict = checkpoint['model_state_dict']
            if ADD_NEW_LAYERS > 0:
                print(f"\nAdding {ADD_NEW_LAYERS} new layers to existing {NUM_LAYERS} layers...")
                model.add_layers(ADD_NEW_LAYERS, NUM_HEADS, FFN_DIM, DROPOUT_RATE, NUM_KV_HEADS)
                print(f"Loading checkpoint with {NUM_LAYERS} layers into model with {NUM_LAYERS + ADD_NEW_LAYERS} layers")
                model.load_state_dict(state_dict, strict=False)
                print(f"New layers initialized as passthrough - they will learn gradually\n")
            else:
                state_dict = {k.replace('_orig_mod.', ''): v for k, v in state_dict.items()}
            
            model.load_state_dict(state_dict, strict=False)
            
            start_epoch = checkpoint.get('epoch', 0) + 1
            total_samples_trained = checkpoint.get('total_samples', 0)
            total_tokens_trained = checkpoint.get('total_tokens', 0)
            
            print(f"Resuming from epoch {start_epoch}")
            print(f"Previous best loss from checkpoint: {checkpoint.get('best_loss', 'N/A')}")
            print(f"Will establish new baseline from first epoch")
            print(f"Total samples trained: {total_samples_trained:,}")
            print(f"Total tokens trained: {total_tokens_trained:,}\n")
        except Exception as e:
            print(f"Failed to load checkpoint: {e}")
            print(f"Starting training from scratch...\n")
            start_epoch = 0
            best_loss = None
            total_samples_trained = 0
            total_tokens_trained = 0
    
    if USE_TORCH_COMPILE:
        print(f"\nCompiling model with mode='{COMPILE_MODE}'...")
        print("This will take 5-10 minutes on first batch, then training will be faster.")
        model = torch.compile(model, mode=COMPILE_MODE)
        print("Model compilation setup complete!\n")
    
    decay_params = []
    no_decay_params = []
    for name, param in model.named_parameters():
        if param.requires_grad:
            if 'norm' in name.lower() or 'embedding' in name.lower():
                no_decay_params.append(param)
            else:
                decay_params.append(param)
    
    print(f"Parameters with weight decay: {sum(p.numel() for p in decay_params):,}")
    print(f"Parameters without weight decay: {sum(p.numel() for p in no_decay_params):,}")
    
    optimizer = torch.optim.AdamW(
        [
            {'params': decay_params, 'weight_decay': WEIGHT_DECAY},
            {'params': no_decay_params, 'weight_decay': 0.0}
        ],
        lr=LEARNING_RATE,
        betas=(0.9, 0.95),
        fused=True if torch.cuda.is_available() else False
    )
    
    steps_per_epoch = 1000 // BATCH_SIZE
    decay_every_n_steps = 2000
    scheduler = get_exp_decay_schedule(optimizer, WARMUP_STEPS, decay_every_n_steps, decay_rate=0.96, min_lr_ratio=0.01)
    
    if LOAD_BEST_MODEL and os.path.exists(WEIGHTS_PATH) and start_epoch > 0:
        try:
            checkpoint = torch.load(WEIGHTS_PATH, map_location=device, weights_only=False)
            if 'optimizer_state_dict' in checkpoint:
                optimizer.load_state_dict(checkpoint['optimizer_state_dict'])
            if 'scheduler_state_dict' in checkpoint:
                scheduler.load_state_dict(checkpoint['scheduler_state_dict'])
        except Exception as e:
            print(f"Failed to load optimizer/scheduler state: {e}")
            print(f"Continuing with fresh optimizer/scheduler...\n")
    
    dataset = TokenDataset(data_gen, BATCH_SIZE, CONTEXT_WINDOW)
    dataloader = DataLoader(dataset, batch_size=None)
    
    scaler = torch.amp.GradScaler('cuda', enabled=(dtype == torch.float16))
    
    model.train()
    
    for epoch in range(start_epoch, EPOCHS):
        print(f"\n{'='*60}")
        print(f"Epoch {epoch + 1}/{EPOCHS}")
        print(f"{'='*60}")
        
        epoch_loss = 0.0
        epoch_samples = 0
        epoch_tokens = 0
        epoch_top1_correct = 0
        epoch_top5_correct = 0
        epoch_total_predictions = 0
        epoch_start_time = time.time()
        
        optimizer.zero_grad()
        
        first_param = None
        for param in model.parameters():
            if param.requires_grad:
                first_param = param.data.clone()
                break
        
        for step, (X_batch, Y_batch, mask_batch, batch_tokens) in enumerate(dataloader):
            if step >= steps_per_epoch:
                break
            
            X_batch = X_batch.to(device)
            Y_batch = Y_batch.to(device)
            mask_batch = mask_batch.to(device)
            
            if step == 0:
                print(f"\nFirst batch stats:")
                print(f"  X_batch shape: {X_batch.shape}")
                print(f"  Y_batch shape: {Y_batch.shape}")
                print(f"  mask_batch shape: {mask_batch.shape}")
                print(f"  mask_batch sum: {mask_batch.sum().item()}")
                print(f"  mask_batch mean: {mask_batch.mean().item():.4f}")
                print(f"  X_batch min/max: {X_batch.min().item()}/{X_batch.max().item()}")
                print(f"  Y_batch min/max: {Y_batch.min().item()}/{Y_batch.max().item()}")
                print(f"  batch_tokens: {batch_tokens}\n")
            
            with torch.amp.autocast('cuda', dtype=dtype):
                logits = model(X_batch)
                
                loss = F.cross_entropy(
                    logits.reshape(-1, vocab_size),
                    Y_batch.reshape(-1),
                    reduction='none'
                )
                
                mask_sum = mask_batch.sum()
                if mask_sum < 1:
                    print(f"WARNING: mask_sum={mask_sum}, skipping batch")
                    continue
                
                loss = (loss * mask_batch.reshape(-1)).sum() / mask_sum.clamp(min=1.0)
                
                with torch.no_grad():
                    _, top1_preds = logits.max(dim=-1)
                    _, top5_preds = logits.topk(5, dim=-1)
                    
                    y_flat = Y_batch.reshape(-1)
                    mask_flat = mask_batch.reshape(-1).bool()
                    top1_flat = top1_preds.reshape(-1)[mask_flat]
                    top5_flat = top5_preds.reshape(-1, 5)[mask_flat]
                    y_masked = y_flat[mask_flat]
                    
                    top1_correct = (top1_flat == y_masked).sum().item()
                    top5_correct = (top5_flat == y_masked.unsqueeze(1)).any(dim=1).sum().item()
                    total_preds = mask_flat.sum().item()
                    
                    epoch_top1_correct += top1_correct
                    epoch_top5_correct += top5_correct
                    epoch_total_predictions += total_preds
            
            scaler.scale(loss).backward()
            scaler.unscale_(optimizer)
            grad_norm = torch.nn.utils.clip_grad_norm_(model.parameters(), MAX_GRAD_NORM)
            scaler.step(optimizer)
            scaler.update()
            optimizer.zero_grad()
            scheduler.step()
            
            batch_samples = X_batch.size(0)
            
            epoch_loss += loss.item()
            epoch_samples += batch_samples
            epoch_tokens += batch_tokens
            total_samples_trained += batch_samples
            total_tokens_trained += batch_tokens
            
            if (step + 1) % 100 == 0:
                current_lr = scheduler.get_last_lr()[0]
                loss_val = loss.item()
                perplexity = math.exp(min(loss_val, 20))
                
                if loss_val > 10:
                    print(f"WARNING: Very high loss {loss_val:.2f} at step {step+1}, mask_sum={batch_tokens}")
                
                if grad_norm < 0.001:
                    print(f"WARNING: Very small grad_norm {grad_norm:.6f} at step {step+1}")
                elif grad_norm > MAX_GRAD_NORM * 0.9:
                    print(f"WARNING: Gradient clipping active, grad_norm={grad_norm:.2f} at step {step+1}")
                
                elapsed_time = time.time() - epoch_start_time
                steps_per_sec = (step + 1) / elapsed_time if elapsed_time > 0 else 0
                top1_acc = 100.0 * top1_correct / total_preds if total_preds > 0 else 0
                top5_acc = 100.0 * top5_correct / total_preds if total_preds > 0 else 0
                epoch_top1_acc = 100.0 * epoch_top1_correct / epoch_total_predictions if epoch_total_predictions > 0 else 0
                epoch_top5_acc = 100.0 * epoch_top5_correct / epoch_total_predictions if epoch_total_predictions > 0 else 0
                
                wandb.log({
                    "step_loss": loss_val,
                    "step_perplexity": perplexity,
                    "step_top1_accuracy": top1_acc,
                    "step_top5_accuracy": top5_acc,
                    "epoch_top1_accuracy": epoch_top1_acc,
                    "epoch_top5_accuracy": epoch_top5_acc,
                    "steps_per_second": steps_per_sec,
                    "learning_rate": current_lr,
                    "grad_norm": grad_norm,
                    "samples_processed": total_samples_trained,
                    "tokens_processed": total_tokens_trained,
                    "epoch_progress": (step + 1) / steps_per_epoch,
                    "global_step": epoch * steps_per_epoch + step + 1
                })
                
                print(f"Step {step + 1}/{steps_per_epoch} | Loss: {loss.item():.4f} | "
                      f"PPL: {perplexity:.2f} | Top1: {top1_acc:.2f}% | Top5: {top5_acc:.2f}% | "
                      f"GradNorm: {grad_norm:.3f} | LR: {current_lr:.2e} | Steps/s: {steps_per_sec:.2f}")
        
        param_change = 0.0
        if first_param is not None:
            for param in model.parameters():
                if param.requires_grad:
                    param_change = (param.data - first_param).abs().max().item()
                    break
        
        print(f"Max parameter change this epoch: {param_change:.6f}")
        
        avg_loss = epoch_loss / steps_per_epoch
        avg_perplexity = math.exp(min(avg_loss, 20))
        epoch_duration = time.time() - epoch_start_time
        steps_per_sec = steps_per_epoch / epoch_duration
        top1_acc = 100.0 * epoch_top1_correct / epoch_total_predictions if epoch_total_predictions > 0 else 0
        top5_acc = 100.0 * epoch_top5_correct / epoch_total_predictions if epoch_total_predictions > 0 else 0
        
        wandb.log({
            "epoch": epoch + 1,
            "loss": avg_loss,
            "perplexity": avg_perplexity,
            "top1_accuracy": top1_acc,
            "top5_accuracy": top5_acc,
            "steps_per_second": steps_per_sec,
            "learning_rate": scheduler.get_last_lr()[0],
            "samples_this_epoch": epoch_samples,
            "tokens_this_epoch": epoch_tokens,
            "total_samples": total_samples_trained,
            "total_tokens": total_tokens_trained,
        })
        
        print(f"\nEpoch {epoch + 1} Summary:")
        print(f"Average Loss: {avg_loss:.4f}")
        print(f"Perplexity: {avg_perplexity:.2f}")
        print(f"Top-1 Accuracy: {top1_acc:.2f}%")
        print(f"Top-5 Accuracy: {top5_acc:.2f}%")
        print(f"Steps/Second: {steps_per_sec:.2f}")
        print(f"Samples: {epoch_samples:,}")
        print(f"Tokens: {epoch_tokens:,}")
        print(f"Total samples trained: {total_samples_trained:,}")
        print(f"Total tokens trained: {total_tokens_trained:,}")
        print(f"Epoch duration: {epoch_duration:.2f}s")
        
        if best_loss is None:
            best_loss = avg_loss
            print(f"\nEstablishing baseline: best_loss set to {best_loss:.4f}")
            print(f"Future epochs will save if they beat this loss\n")
            
            wandb.log({
                "baseline_established": True,
                "baseline_loss": best_loss,
                "baseline_epoch": epoch + 1
            })
        
        if avg_loss < best_loss:
            best_loss = avg_loss
            
            model_state = model._orig_mod.state_dict() if hasattr(model, '_orig_mod') else model.state_dict()
            
            checkpoint = {
                'epoch': epoch,
                'model_state_dict': model_state,
                'optimizer_state_dict': optimizer.state_dict(),
                'scheduler_state_dict': scheduler.state_dict(),
                'best_loss': best_loss,
                'total_samples': total_samples_trained,
                'total_tokens': total_tokens_trained,
                'config': wandb.config.as_dict(),
                'wandb_run_id': wandb.run.id
            }
            torch.save(checkpoint, WEIGHTS_PATH)
            print(f"New best model saved with loss: {best_loss:.4f}")
            
            wandb.log({
                "model_saved": True,
                "saved_epoch": epoch + 1,
                "saved_loss": best_loss,
                "saved_perplexity": avg_perplexity,
                "saved_top1_accuracy": top1_acc,
                "saved_top5_accuracy": top5_acc
            })
    
    wandb.finish()


if __name__ == '__main__':
    train()
