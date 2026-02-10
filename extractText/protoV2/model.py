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
    def __init__(self, embed_dim, vocab_size, embedding_weight, d_model=1024, n_layers=24, n_heads=16, ffn_mult=4, dropout=0.1):
        super().__init__()
        self.vocab_size = vocab_size
        self.d_model = d_model
        self.embed_dim = embed_dim
        self.adapter = nn.Linear(embed_dim, d_model)
        self.blocks = nn.ModuleList([
            TransformerBlock(d_model, n_heads, ffn_mult, dropout) for _ in range(n_layers)
        ])
        self.ln_f = nn.LayerNorm(d_model)
        self.out_proj = nn.Linear(d_model, embed_dim, bias=False)
        self.register_buffer("embedding_weight", embedding_weight, persistent=False)

    def forward(self, input_embeds):
        x = input_embeds
        x = self.adapter(x)
        seq_len = input_embeds.size(1)
        attn_mask = torch.triu(torch.ones(seq_len, seq_len, device=input_embeds.device), diagonal=1).bool()
        for block in self.blocks:
            x = block(x, attn_mask)
        x = self.ln_f(x)
        x = self.out_proj(x)
        logits = torch.matmul(x, self.embedding_weight.t())
        return logits

    def count_parameters(self):
        return sum(p.numel() for p in self.parameters())
