import torch
from torch import nn

class TransformerBlock(nn.Module):
    def __init__(self, d_model, n_heads, ffn_mult, dropout):
        super().__init__()
        self.ln1 = nn.LayerNorm(d_model)
        self.attn = nn.MultiheadAttention(d_model, n_heads, dropout=dropout, batch_first=True)
        self.ln2 = nn.LayerNorm(d_model)
        self.ffn = nn.Sequential(
            nn.Linear(d_model, d_model * ffn_mult),
            nn.GELU(),
            nn.Linear(d_model * ffn_mult, d_model)
        )
        self.dropout = nn.Dropout(dropout)

    def forward(self, x, attn_mask):
        h = self.ln1(x)
        attn_out, _ = self.attn(h, h, h, attn_mask=attn_mask, need_weights=False)
        x = x + self.dropout(attn_out)
        h = self.ln2(x)
        x = x + self.dropout(self.ffn(h))
        return x

class SmallTransformerLM(nn.Module):
    def __init__(self, vocab_size, embedding_weight, n_layers=24, n_heads=16, ffn_mult=4, dropout=0.1):
        super().__init__()
        self.vocab_size = vocab_size
        self.d_model = embedding_weight.size(1)
        self.token_embed = nn.Embedding.from_pretrained(embedding_weight, freeze=True)
        self.blocks = nn.ModuleList([
            TransformerBlock(self.d_model, n_heads, ffn_mult, dropout) for _ in range(n_layers)
        ])
        self.ln_f = nn.LayerNorm(self.d_model)

    def forward(self, input_ids):
        x = self.token_embed(input_ids)
        seq_len = input_ids.size(1)
        attn_mask = torch.triu(torch.ones(seq_len, seq_len, device=input_ids.device), diagonal=1).bool()
        for block in self.blocks:
            x = block(x, attn_mask)
        x = self.ln_f(x)
        logits = torch.matmul(x, self.token_embed.weight.t())
        return logits

    def count_parameters(self):
        return sum(p.numel() for p in self.parameters())
