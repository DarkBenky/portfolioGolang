import os
import torch
import torch.nn as nn
import torch.nn.functional as F
from torch.utils.data import IterableDataset, DataLoader
import wandb
from dataGen import DataGenerator
import math
from einops import rearrange

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
        
        if seq_len != self.seq_len_cached:
            self.seq_len_cached = seq_len
            t = torch.arange(seq_len, device=x.device, dtype=self.inv_freq.dtype)
            freqs = torch.einsum("i,j->ij", t, self.inv_freq)
            emb = torch.cat((freqs, freqs), dim=-1).to(x.device)
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
    def __init__(self, d_model, num_heads, dropout=0.0, bias=False):
        super().__init__()
        assert d_model % num_heads == 0
        self.d_model = d_model
        self.num_heads = num_heads
        self.head_dim = d_model // num_heads
        
        self.qkv = nn.Linear(d_model, 3 * d_model, bias=bias)
        self.proj = nn.Linear(d_model, d_model, bias=bias)
        self.dropout = dropout
        self.rotary = RotaryEmbedding(self.head_dim)

    def forward(self, x):
        batch_size, seq_len, _ = x.shape
        
        qkv = self.qkv(x)
        qkv = rearrange(qkv, 'b s (three h d) -> b s three h d', three=3, h=self.num_heads)
        
        q, k, v = qkv[:, :, 0], qkv[:, :, 1], qkv[:, :, 2]
        
        cos, sin = self.rotary(x, seq_len)
        q, k = apply_rotary_pos_emb(q, k, cos, sin)
        
        if FLASH_AVAILABLE and x.dtype in [torch.float16, torch.bfloat16]:
            qkv_for_flash = torch.stack([q, k, v], dim=2)
            out = flash_attn_qkvpacked_func(
                qkv_for_flash,
                dropout_p=self.dropout if self.training else 0.0,
                causal=True
            )
        else:
            q = q.transpose(1, 2)
            k = k.transpose(1, 2)
            v = v.transpose(1, 2)
            
            scale = 1.0 / math.sqrt(self.head_dim)
            attn = torch.matmul(q, k.transpose(-2, -1)) * scale
            
            causal_mask = torch.triu(torch.ones(seq_len, seq_len, device=x.device), diagonal=1).bool()
            attn = attn.masked_fill(causal_mask, float('-inf'))
            
            attn = F.softmax(attn, dim=-1)
            attn = F.dropout(attn, p=self.dropout, training=self.training)
            
            out = torch.matmul(attn, v)
            out = out.transpose(1, 2)
        
        out = rearrange(out, 'b s h d -> b s (h d)')
        return self.proj(out)


class TransformerBlock(nn.Module):
    def __init__(self, d_model, num_heads, ff_dim, dropout=0.0, layer_idx=0, num_layers=1):
        super().__init__()
        self.attention = FlashAttentionBlock(d_model, num_heads, dropout)
        self.feed_forward = SwiGLU(d_model, ff_dim, dropout)
        self.norm1 = RMSNorm(d_model)
        self.norm2 = RMSNorm(d_model)
        self.dropout = nn.Dropout(dropout)
        self.scale = math.sqrt(num_layers)

    def forward(self, x):
        residual = x
        x = self.norm1(x)
        x = residual + self.dropout(self.attention(x)) / self.scale
        
        residual = x
        x = self.norm2(x)
        x = residual + self.dropout(self.feed_forward(x)) / self.scale
        
        return x


class CausalLanguageModel(nn.Module):
    def __init__(self, vocab_size, context_window, d_model, num_heads, num_layers, ff_dim, dropout=0.0):
        super().__init__()
        self.vocab_size = vocab_size
        self.d_model = d_model
        
        self.token_embedding = nn.Embedding(vocab_size, d_model)
        self.dropout = nn.Dropout(dropout)
        
        self.blocks = nn.ModuleList([
            TransformerBlock(d_model, num_heads, ff_dim, dropout, i, num_layers)
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

    def forward(self, x):
        x = self.token_embedding(x)
        x = self.dropout(x)
        
        for block in self.blocks:
            x = block(x)
        
        x = self.final_norm(x)
        
        logits = F.linear(x, self.token_embedding.weight)
        
        return logits

    def count_parameters(self):
        return sum(p.numel() for p in self.parameters() if p.requires_grad)


class TokenDataset(IterableDataset):
    def __init__(self, data_generator, batch_size, context_window):
        self.data_generator = data_generator
        self.batch_size = batch_size
        self.context_window = context_window

    def __iter__(self):
        for X_batch, Y_batch, mask_batch in self.data_generator.generate_batches(
            self.batch_size, self.context_window
        ):
            X_batch = torch.from_numpy(X_batch).long()
            Y_batch = torch.from_numpy(Y_batch).long()
            mask_batch = torch.from_numpy(mask_batch).float()
            yield X_batch, Y_batch, mask_batch


def get_cosine_schedule_with_warmup(optimizer, num_warmup_steps, num_training_steps, min_lr_ratio=0.1):
    def lr_lambda(current_step):
        if current_step < num_warmup_steps:
            return float(current_step) / float(max(1, num_warmup_steps))
        progress = float(current_step - num_warmup_steps) / float(max(1, num_training_steps - num_warmup_steps))
        return max(min_lr_ratio, 0.5 * (1.0 + math.cos(math.pi * progress)))
    
    return torch.optim.lr_scheduler.LambdaLR(optimizer, lr_lambda)


def train():
    CONTEXT_WINDOW = 4096
    D_MODEL = 1536
    NUM_HEADS = 24
    NUM_LAYERS = 64
    BATCH_SIZE = 1
    GRAD_ACCUM_STEPS = 8
    EFFECTIVE_BATCH_SIZE = BATCH_SIZE * GRAD_ACCUM_STEPS
    LEARNING_RATE = 3e-4
    WEIGHT_DECAY = 0.1
    WARMUP_STEPS = 2000
    EPOCHS = 10000
    FFN_DIM = int(D_MODEL * 8 / 3)
    DROPOUT_RATE = 0.0
    WEIGHTS_PATH = 'best_model_pytorch.pt'
    LOAD_BEST_MODEL = True
    MAX_GRAD_NORM = 1.0
    
    device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
    dtype = torch.bfloat16 if torch.cuda.is_available() and torch.cuda.is_bf16_supported() else torch.float16
    
    print(f"Using device: {device}")
    print(f"Using dtype: {dtype}")
    print(f"FlashAttention available: {FLASH_AVAILABLE}")
    
    wandb.init(
        project="portfolio-transformer-v2",
        config={
            "context_window": CONTEXT_WINDOW,
            "d_model": D_MODEL,
            "num_heads": NUM_HEADS,
            "num_layers": NUM_LAYERS,
            "batch_size": BATCH_SIZE,
            "grad_accum_steps": GRAD_ACCUM_STEPS,
            "effective_batch_size": EFFECTIVE_BATCH_SIZE,
            "learning_rate": LEARNING_RATE,
            "weight_decay": WEIGHT_DECAY,
            "warmup_steps": WARMUP_STEPS,
            "epochs": EPOCHS,
            "ffn_dim": FFN_DIM,
            "dropout_rate": DROPOUT_RATE,
            "dtype": str(dtype),
            "flash_attention": FLASH_AVAILABLE,
            "improvements": "RMSNorm + SwiGLU + RoPE + FlashAttention2 + WeightTying + GradAccum",
            "load_best_model": LOAD_BEST_MODEL
        }
    )
    
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
        dropout=DROPOUT_RATE
    )
    
    model = model.to(device)
    
    total_params = model.count_parameters()
    print(f"\nTotal parameters: {total_params:,}")
    print(f"Model size: ~{total_params * 2 / 1e9:.2f} GB (bfloat16)")
    
    start_epoch = 0
    best_loss = float('inf')
    total_samples_trained = 0
    total_tokens_trained = 0
    
    if LOAD_BEST_MODEL and os.path.exists(WEIGHTS_PATH):
        print(f"\n{'='*60}")
        print(f"LOADING MODEL from {WEIGHTS_PATH}")
        print(f"{'='*60}\n")
        
        checkpoint = torch.load(WEIGHTS_PATH, map_location=device)
        model.load_state_dict(checkpoint['model_state_dict'])
        best_loss = checkpoint.get('best_loss', float('inf'))
        start_epoch = checkpoint.get('epoch', 0) + 1
        total_samples_trained = checkpoint.get('total_samples', 0)
        total_tokens_trained = checkpoint.get('total_tokens', 0)
        
        print(f"Resuming from epoch {start_epoch}")
        print(f"Best loss so far: {best_loss:.4f}")
        print(f"Total samples trained: {total_samples_trained:,}")
        print(f"Total tokens trained: {total_tokens_trained:,}\n")
    
    optimizer = torch.optim.AdamW(
        model.parameters(),
        lr=LEARNING_RATE,
        betas=(0.9, 0.95),
        weight_decay=WEIGHT_DECAY,
        fused=True if torch.cuda.is_available() else False
    )
    
    steps_per_epoch = 1000 // BATCH_SIZE
    total_steps = EPOCHS * steps_per_epoch
    scheduler = get_cosine_schedule_with_warmup(optimizer, WARMUP_STEPS, total_steps)
    
    if LOAD_BEST_MODEL and os.path.exists(WEIGHTS_PATH):
        checkpoint = torch.load(WEIGHTS_PATH, map_location=device)
        if 'optimizer_state_dict' in checkpoint:
            optimizer.load_state_dict(checkpoint['optimizer_state_dict'])
        if 'scheduler_state_dict' in checkpoint:
            scheduler.load_state_dict(checkpoint['scheduler_state_dict'])
    
    dataset = TokenDataset(data_gen, BATCH_SIZE, CONTEXT_WINDOW)
    dataloader = DataLoader(dataset, batch_size=None)
    
    scaler = torch.cuda.amp.GradScaler(enabled=(dtype == torch.float16))
    
    model.train()
    
    for epoch in range(start_epoch, EPOCHS):
        print(f"\n{'='*60}")
        print(f"Epoch {epoch + 1}/{EPOCHS}")
        print(f"{'='*60}")
        
        epoch_loss = 0.0
        epoch_samples = 0
        epoch_tokens = 0
        
        optimizer.zero_grad()
        
        for step, (X_batch, Y_batch, mask_batch) in enumerate(dataloader):
            if step >= steps_per_epoch:
                break
            
            X_batch = X_batch.to(device)
            Y_batch = Y_batch.to(device)
            mask_batch = mask_batch.to(device)
            
            with torch.cuda.amp.autocast(dtype=dtype):
                logits = model(X_batch)
                
                loss = F.cross_entropy(
                    logits.reshape(-1, vocab_size),
                    Y_batch.reshape(-1),
                    reduction='none'
                )
                loss = (loss * mask_batch.reshape(-1)).sum() / mask_batch.sum()
                loss = loss / GRAD_ACCUM_STEPS
            
            scaler.scale(loss).backward()
            
            if (step + 1) % GRAD_ACCUM_STEPS == 0:
                scaler.unscale_(optimizer)
                torch.nn.utils.clip_grad_norm_(model.parameters(), MAX_GRAD_NORM)
                scaler.step(optimizer)
                scaler.update()
                optimizer.zero_grad()
                scheduler.step()
            
            batch_samples = X_batch.size(0)
            batch_tokens = int(mask_batch.sum().item())
            
            epoch_loss += loss.item() * GRAD_ACCUM_STEPS
            epoch_samples += batch_samples
            epoch_tokens += batch_tokens
            total_samples_trained += batch_samples
            total_tokens_trained += batch_tokens
            
            if (step + 1) % 100 == 0:
                current_lr = scheduler.get_last_lr()[0]
                perplexity = math.exp(min(loss.item() * GRAD_ACCUM_STEPS, 20))
                
                print(f"Step {step + 1}/{steps_per_epoch} | Loss: {loss.item() * GRAD_ACCUM_STEPS:.4f} | "
                      f"PPL: {perplexity:.2f} | LR: {current_lr:.2e}")
        
        avg_loss = epoch_loss / steps_per_epoch
        avg_perplexity = math.exp(min(avg_loss, 20))
        
        wandb.log({
            "epoch": epoch + 1,
            "loss": avg_loss,
            "perplexity": avg_perplexity,
            "learning_rate": scheduler.get_last_lr()[0],
            "samples_this_epoch": epoch_samples,
            "tokens_this_epoch": epoch_tokens,
            "total_samples": total_samples_trained,
            "total_tokens": total_tokens_trained,
        })
        
        print(f"\nEpoch {epoch + 1} Summary:")
        print(f"Average Loss: {avg_loss:.4f}")
        print(f"Perplexity: {avg_perplexity:.2f}")
        print(f"Samples: {epoch_samples:,}")
        print(f"Tokens: {epoch_tokens:,}")
        print(f"Total samples trained: {total_samples_trained:,}")
        print(f"Total tokens trained: {total_tokens_trained:,}")
        
        if avg_loss < best_loss:
            best_loss = avg_loss
            checkpoint = {
                'epoch': epoch,
                'model_state_dict': model.state_dict(),
                'optimizer_state_dict': optimizer.state_dict(),
                'scheduler_state_dict': scheduler.state_dict(),
                'best_loss': best_loss,
                'total_samples': total_samples_trained,
                'total_tokens': total_tokens_trained,
                'config': wandb.config.as_dict()
            }
            torch.save(checkpoint, WEIGHTS_PATH)
            print(f"New best model saved with loss: {best_loss:.4f}")
    
    wandb.finish()


if __name__ == '__main__':
    train()
