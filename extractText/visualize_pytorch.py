import os
import torch
import torch.nn as nn
import torch.nn.functional as F
import numpy as np
from tokenizers import Tokenizer
from train_pytorch import CausalLanguageModel, FLASH_AVAILABLE
import plotly.graph_objects as go
from plotly.subplots import make_subplots
from sklearn.manifold import TSNE
from sklearn.decomposition import PCA


class ModelVisualizer:
    def __init__(self, weights_path, tokenizer_path, context_window=2048, d_model=2048, num_heads=32, num_layers=18, num_kv_heads=8, dropout_rate=0.0):
        self.device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
        self.tokenizer = Tokenizer.from_file(tokenizer_path)
        self.vocab_size = self.tokenizer.get_vocab_size()
        self.num_heads = num_heads
        self.num_layers = num_layers
        self.d_model = d_model
        
        print(f"Building PyTorch model architecture...")
        self.model = CausalLanguageModel(
            vocab_size=self.vocab_size,
            context_window=context_window,
            d_model=d_model,
            num_heads=num_heads,
            num_layers=num_layers,
            ff_dim=int(d_model * 8 / 3),
            dropout=dropout_rate,
            num_kv_heads=num_kv_heads
        )
        
        print(f"Loading weights from {weights_path}...")
        checkpoint = torch.load(weights_path, map_location=self.device, weights_only=False)
        state_dict = checkpoint.get('model_state_dict', checkpoint)
        state_dict = {k.replace('_orig_mod.', ''): v for k, v in state_dict.items()}
        self.model.load_state_dict(state_dict, strict=False)
        self.model.to(self.device)
        self.model.eval()
        
        total_params = sum(p.numel() for p in self.model.parameters() if p.requires_grad)
        print(f"Model loaded! Total parameters: {total_params:,}\n")
        
        self.embedding_layer = self.model.token_embedding
    
    def extract_attention_weights(self, text, layer_idx=0):
        tokens = self.tokenizer.encode(text).ids
        input_seq = torch.tensor([tokens], dtype=torch.long, device=self.device)
        
        if layer_idx >= len(self.model.blocks):
            raise ValueError(f"Layer {layer_idx} not found. Model has {len(self.model.blocks)} blocks.")
        
        attention_weights = []
        
        def hook_fn(module, input, output):
            if hasattr(module, '_attn_weights'):
                attention_weights.append(module._attn_weights.detach().cpu())
        
        target_block = self.model.blocks[layer_idx]
        handle = target_block.attention.register_forward_hook(hook_fn)
        
        try:
            with torch.no_grad():
                x = self.model.token_embedding(input_seq)
                x = self.model.dropout(x)
                
                for i, block in enumerate(self.model.blocks):
                    if i == layer_idx:
                        residual = x
                        x = block.norm1(x)
                        batch_size, seq_len, _ = x.shape
                        
                        attn = block.attention
                        q = attn.q_proj(x)
                        k = attn.k_proj(x)
                        v = attn.v_proj(x)
                        
                        q = q.view(batch_size, seq_len, attn.num_heads, attn.head_dim)
                        k = k.view(batch_size, seq_len, attn.num_kv_heads, attn.head_dim)
                        v = v.view(batch_size, seq_len, attn.num_kv_heads, attn.head_dim)
                        
                        cos, sin = attn.rotary(x, seq_len)
                        q, k = apply_rotary_pos_emb(q, k, cos, sin)
                        
                        if not FLASH_AVAILABLE or x.dtype not in [torch.float16, torch.bfloat16]:
                            if attn.num_kv_heads != attn.num_heads:
                                k = k.repeat_interleave(attn.num_queries_per_kv, dim=2)
                                v = v.repeat_interleave(attn.num_queries_per_kv, dim=2)
                            
                            q = q.transpose(1, 2)
                            k = k.transpose(1, 2)
                            v = v.transpose(1, 2)
                            
                            attn_scores = torch.matmul(q, k.transpose(-2, -1)) / (attn.head_dim ** 0.5)
                            causal_mask = torch.triu(torch.ones(seq_len, seq_len, device=x.device), diagonal=1).bool()
                            attn_scores = attn_scores.masked_fill(causal_mask, float('-inf'))
                            attn_weights_tensor = F.softmax(attn_scores, dim=-1)
                            attention_weights.append(attn_weights_tensor.detach().cpu())
                        
                        break
                    else:
                        x = block(x)
        finally:
            handle.remove()
        
        if attention_weights:
            return attention_weights[0].numpy(), tokens
        return None, tokens
    
    def visualize_attention(self, text, layer_idx=0, head_idx=None, save_path=None):
        attention_weights, tokens = self.extract_attention_weights(text, layer_idx)
        
        token_labels = [self.tokenizer.decode([t]) for t in tokens]
        
        if attention_weights is None:
            print("Attention extraction not available. Showing token embeddings instead.")
            self.visualize_embeddings(tokens[:20], save_path=save_path)
            return
        
        if head_idx is not None:
            attention_weights = attention_weights[0, head_idx, :, :]
            
            fig = go.Figure(data=go.Heatmap(
                z=attention_weights,
                x=token_labels,
                y=token_labels,
                colorscale='Viridis',
                hoverongaps=False,
                colorbar=dict(title="Attention<br>Weight")
            ))
            
            fig.update_layout(
                title=dict(
                    text=f'Attention Pattern - Layer {layer_idx}, Head {head_idx}<br><sub>Darker = Stronger Attention</sub>',
                    x=0.5,
                    xanchor='center'
                ),
                xaxis_title='Keys',
                yaxis_title='Queries',
                width=900,
                height=900,
                font=dict(size=12)
            )
        else:
            num_heads = attention_weights.shape[1]
            cols = min(4, num_heads)
            rows = (num_heads + cols - 1) // cols
            
            fig = make_subplots(
                rows=rows, cols=cols,
                subplot_titles=[f'Head {i}' for i in range(num_heads)],
                vertical_spacing=0.05,
                horizontal_spacing=0.05
            )
            
            for head in range(num_heads):
                row = head // cols + 1
                col = head % cols + 1
                
                fig.add_trace(
                    go.Heatmap(
                        z=attention_weights[0, head, :, :],
                        x=token_labels,
                        y=token_labels,
                        colorscale='Viridis',
                        showscale=(col == cols),
                        hoverongaps=False
                    ),
                    row=row, col=col
                )
            
            fig.update_layout(
                title=dict(
                    text=f'Multi-Head Attention Patterns - Layer {layer_idx}<br><sub>Showing all {num_heads} attention heads</sub>',
                    x=0.5,
                    xanchor='center'
                ),
                width=1400,
                height=350 * rows,
                showlegend=False,
                font=dict(size=10)
            )
        
        if save_path:
            fig.write_html(save_path)
            print(f"Saved to {save_path}")
        else:
            fig.show()
    
    def visualize_embeddings(self, tokens, method='tsne', sample_size=1000, save_path=None, use_3d=False):
        embeddings = self.embedding_layer.weight.detach().cpu().numpy()
        
        if len(embeddings) > sample_size:
            indices = np.random.choice(len(embeddings), sample_size, replace=False)
            embeddings_sample = embeddings[indices]
        else:
            indices = np.arange(len(embeddings))
            embeddings_sample = embeddings
        
        n_components = 3 if use_3d else 2
        
        if method == 'tsne':
            reducer = TSNE(n_components=n_components, random_state=42, perplexity=min(30, len(embeddings_sample)-1))
        else:
            reducer = PCA(n_components=n_components, random_state=42)
        
        embeddings_reduced = reducer.fit_transform(embeddings_sample)
        
        labels = [self.tokenizer.decode([int(i)]) for i in indices]
        
        highlight = []
        if isinstance(tokens, list):
            highlight = [i for i, idx in enumerate(indices) if idx in tokens]
        
        fig = go.Figure()
        
        if use_3d:
            fig.add_trace(go.Scatter3d(
                x=embeddings_reduced[:, 0],
                y=embeddings_reduced[:, 1],
                z=embeddings_reduced[:, 2],
                mode='markers',
                marker=dict(size=3, opacity=0.6, color='lightblue'),
                text=labels,
                hovertemplate='Token: <b>%{text}</b><br>X: %{x:.2f}<br>Y: %{y:.2f}<br>Z: %{z:.2f}<extra></extra>'
            ))
            
            if highlight:
                fig.add_trace(go.Scatter3d(
                    x=embeddings_reduced[highlight, 0],
                    y=embeddings_reduced[highlight, 1],
                    z=embeddings_reduced[highlight, 2],
                    mode='markers+text',
                    marker=dict(size=8, color='red', symbol='diamond'),
                    text=[labels[i] for i in highlight],
                    textposition='top center',
                    name='Highlighted',
                    hovertemplate='Token: <b>%{text}</b><extra></extra>'
                ))
            
            fig.update_layout(
                title=f'3D Token Embeddings ({method.upper()})<br><sub>{len(embeddings_sample)} tokens visualized</sub>',
                scene=dict(
                    xaxis_title=f'{method.upper()} Component 1',
                    yaxis_title=f'{method.upper()} Component 2',
                    zaxis_title=f'{method.upper()} Component 3',
                ),
                width=1200,
                height=900
            )
        else:
            fig.add_trace(go.Scatter(
                x=embeddings_reduced[:, 0],
                y=embeddings_reduced[:, 1],
                mode='markers',
                marker=dict(size=4, opacity=0.6, color='lightblue'),
                text=labels,
                hovertemplate='Token: <b>%{text}</b><br>X: %{x:.2f}<br>Y: %{y:.2f}<extra></extra>'
            ))
            
            if highlight:
                fig.add_trace(go.Scatter(
                    x=embeddings_reduced[highlight, 0],
                    y=embeddings_reduced[highlight, 1],
                    mode='markers+text',
                    marker=dict(size=10, color='red', symbol='diamond'),
                    text=[labels[i] for i in highlight],
                    textposition='top center',
                    name='Highlighted',
                    hovertemplate='Token: <b>%{text}</b><extra></extra>'
                ))
            
            fig.update_layout(
                title=f'2D Token Embeddings ({method.upper()})<br><sub>{len(embeddings_sample)} tokens visualized</sub>',
                xaxis_title=f'{method.upper()} Component 1',
                yaxis_title=f'{method.upper()} Component 2',
                width=1200,
                height=900
            )
        
        if save_path:
            fig.write_html(save_path)
            print(f"Saved to {save_path}")
        else:
            fig.show()
    
    def visualize_token_similarities(self, tokens, top_k=10, save_path=None):
        embeddings = self.embedding_layer.weight.detach().cpu().numpy()
        
        if isinstance(tokens, str):
            tokens = self.tokenizer.encode(tokens).ids
        
        token_labels = [self.tokenizer.decode([t]) for t in tokens]
        
        similarity_matrix = []
        similar_token_labels = []
        
        for token in tokens:
            token_emb = embeddings[token]
            similarities = np.dot(embeddings, token_emb) / (np.linalg.norm(embeddings, axis=1) * np.linalg.norm(token_emb))
            top_indices = np.argpartition(similarities, -top_k)[-top_k:]
            top_indices = top_indices[np.argsort(-similarities[top_indices])]
            
            similarity_matrix.append(similarities[top_indices])
            if len(similar_token_labels) == 0:
                similar_token_labels = [self.tokenizer.decode([int(i)]) for i in top_indices]
        
        similarity_matrix = np.array(similarity_matrix)
        
        fig = go.Figure(data=go.Heatmap(
            z=similarity_matrix,
            x=similar_token_labels,
            y=token_labels,
            colorscale='RdYlGn',
            text=np.round(similarity_matrix, 3),
            texttemplate='%{text}',
            textfont={"size": 10},
            colorbar=dict(title="Cosine<br>Similarity"),
            hoverongaps=False,
            hovertemplate='Query: <b>%{y}</b><br>Similar: <b>%{x}</b><br>Similarity: %{z:.4f}<extra></extra>'
        ))
        
        fig.update_layout(
            title=dict(
                text=f'Token Similarity Matrix<br><sub>Top {top_k} most similar tokens by cosine similarity</sub>',
                x=0.5,
                xanchor='center'
            ),
            xaxis_title='Most Similar Tokens',
            yaxis_title='Query Tokens',
            width=1200,
            height=max(600, 100 + len(tokens) * 50),
            font=dict(size=11),
            xaxis=dict(tickangle=45)
        )
        
        if save_path:
            fig.write_html(save_path)
            print(f"Saved to {save_path}")
        else:
            fig.show()
    
    def search_tokens(self, search_query, max_results=100, save_path=None):
        print(f"Searching for tokens matching: '{search_query}'...")
        
        matching_indices = []
        matching_labels = []
        
        for i in range(self.vocab_size):
            token_str = self.tokenizer.decode([i])
            if search_query.lower() in token_str.lower():
                matching_indices.append(i)
                matching_labels.append(token_str)
                if len(matching_indices) >= max_results:
                    break
        
        if not matching_indices:
            print(f"No tokens found matching '{search_query}'")
            return
        
        print(f"Found {len(matching_indices)} matching tokens")
        
        embeddings = self.embedding_layer.weight.detach().cpu().numpy()
        matching_embeddings = embeddings[matching_indices]
        
        use_3d = len(matching_indices) >= 3
        n_components = 3 if use_3d else min(2, len(matching_indices))
        
        if len(matching_indices) < 3:
            print("Not enough matching tokens for visualization")
        
        reducer = PCA(n_components=n_components, random_state=42)
        embeddings_reduced = reducer.fit_transform(matching_embeddings)
        
        if use_3d:
            fig = go.Figure(data=go.Scatter3d(
                x=embeddings_reduced[:, 0],
                y=embeddings_reduced[:, 1],
                z=embeddings_reduced[:, 2],
                mode='markers+text',
                marker=dict(size=6, color='blue', opacity=0.7),
                text=matching_labels,
                textposition='top center',
                hovertemplate='Token: <b>%{text}</b><extra></extra>'
            ))
            
            fig.update_layout(
                title=f'Tokens matching "{search_query}" (3D PCA)<br><sub>{len(matching_indices)} tokens found</sub>',
                scene=dict(
                    xaxis_title='PC1',
                    yaxis_title='PC2',
                    zaxis_title='PC3'
                ),
                width=1200,
                height=900
            )
        else:
            fig = go.Figure(data=go.Scatter(
                x=embeddings_reduced[:, 0],
                y=embeddings_reduced[:, 1],
                mode='markers+text',
                marker=dict(size=8, color='blue', opacity=0.7),
                text=matching_labels,
                textposition='top center',
                hovertemplate='Token: <b>%{text}</b><extra></extra>'
            ))
            
            fig.update_layout(
                title=f'Tokens matching "{search_query}" (2D PCA)<br><sub>{len(matching_indices)} tokens found</sub>',
                xaxis_title='PC1',
                yaxis_title='PC2',
                width=1200,
                height=900
            )
        
        if save_path:
            fig.write_html(save_path)
            print(f"Saved to {save_path}")
        else:
            fig.show()
    
    def visualize_layer_outputs(self, text, save_path=None):
        tokens = self.tokenizer.encode(text).ids
        input_seq = torch.tensor([tokens], dtype=torch.long, device=self.device)
        
        token_labels = [self.tokenizer.decode([t]) for t in tokens]
        
        with torch.no_grad():
            embedding_output = self.embedding_layer(input_seq).cpu().numpy()
            predictions = self.model(input_seq).cpu().numpy()
        
        fig = make_subplots(
            rows=2, cols=1,
            subplot_titles=['Token Embeddings (First 50 dimensions)', 'Model Output Logits (Top 50 tokens)'],
            vertical_spacing=0.15,
            row_heights=[0.5, 0.5]
        )
        
        embedding_2d = embedding_output[0, :, :min(50, embedding_output.shape[-1])]
        
        fig.add_trace(
            go.Heatmap(
                z=embedding_2d.T,
                x=token_labels,
                y=[f'Dim {i}' for i in range(embedding_2d.shape[1])],
                colorscale='Viridis',
                showscale=True,
                hoverongaps=False,
                colorbar=dict(title="Value", x=1.02)
            ),
            row=1, col=1
        )
        
        output_2d = predictions[0, :, :min(50, predictions.shape[-1])]
        
        fig.add_trace(
            go.Heatmap(
                z=output_2d.T,
                x=token_labels,
                y=[f'Token {i}' for i in range(output_2d.shape[1])],
                colorscale='RdBu',
                showscale=True,
                hoverongaps=False,
                colorbar=dict(title="Logit", x=1.02)
            ),
            row=2, col=1
        )
        
        fig.update_xaxes(title_text="Input Tokens", row=1, col=1)
        fig.update_xaxes(title_text="Input Tokens", row=2, col=1)
        fig.update_yaxes(title_text="Embedding Dimensions", row=1, col=1)
        fig.update_yaxes(title_text="Output Vocabulary", row=2, col=1)
        
        fig.update_layout(
            title_text='Model Representations: Embeddings & Output Logits',
            width=1400,
            height=900,
            showlegend=False
        )
        
        if save_path:
            fig.write_html(save_path)
            print(f"Saved to {save_path}")
        else:
            fig.show()


def apply_rotary_pos_emb(q, k, cos, sin):
    def rotate_half(x):
        x1, x2 = x[..., : x.shape[-1] // 2], x[..., x.shape[-1] // 2 :]
        return torch.cat((-x2, x1), dim=-1)
    
    q_embed = (q * cos) + (rotate_half(q) * sin)
    k_embed = (k * cos) + (rotate_half(k) * sin)
    return q_embed, k_embed


def main():
    output_dir = 'visualizations'
    os.makedirs(output_dir, exist_ok=True)
    
    print("Initializing PyTorch visualizer...")
    visualizer = ModelVisualizer(
        weights_path='best_model_pytorch_v2.pt',
        tokenizer_path='tokenizer.json',
        context_window=2048,
        d_model=2048,
        num_heads=32,
        num_layers=18,
        num_kv_heads=8,
        dropout_rate=0.0
    )
    
    sample_text = """import requests
import re

def crawl_website_for_phone_numbers(website):
    response = requests.get(website)
    phone_numbers = re.findall('\d{3}-\d{3}-\d{4}', response.text)
    return phone_numbers
    
if __name__ == '__main__':
    print(crawl_website_for_phone_numbers('www.example.com'))"""
    
    print("\nGenerating visualizations...")
    print(f"Sample text: {sample_text}")
    print(f"Output directory: {output_dir}/")
    
    print("\n1. Generating attention patterns (Layer 0, all heads)...")
    visualizer.visualize_attention(sample_text, layer_idx=0, head_idx=None, save_path=f'{output_dir}/attention_all_heads.html')
    
    print("\n2. Generating attention patterns (Layer 0, Head 0)...")
    visualizer.visualize_attention(sample_text, layer_idx=0, head_idx=0, save_path=f'{output_dir}/attention_layer0_head0.html')
    
    print(f"\n3. Generating 2D token embeddings (t-SNE)...")
    tokens = visualizer.tokenizer.encode(sample_text).ids
    visualizer.visualize_embeddings(tokens, method='tsne', sample_size=min(1000, visualizer.vocab_size), save_path=f'{output_dir}/embeddings_tsne_2d.html', use_3d=False)
    
    print(f"\n4. Generating 3D token embeddings (t-SNE)...")
    visualizer.visualize_embeddings(tokens, method='tsne', sample_size=min(1000, visualizer.vocab_size), save_path=f'{output_dir}/embeddings_tsne_3d.html', use_3d=True)
    
    print(f"\n5. Generating 2D token embeddings (PCA)...")
    visualizer.visualize_embeddings(tokens, method='pca', sample_size=min(1000, visualizer.vocab_size), save_path=f'{output_dir}/embeddings_pca_2d.html', use_3d=False)
    
    print(f"\n6. Generating 3D token embeddings (PCA)...")
    visualizer.visualize_embeddings(tokens, method='pca', sample_size=min(1000, visualizer.vocab_size), save_path=f'{output_dir}/embeddings_pca_3d.html', use_3d=True)
    
    print("\n7. Generating token similarities...")
    visualizer.visualize_token_similarities(sample_text, top_k=24, save_path=f'{output_dir}/token_similarities.html')
    
    print("\n8. Searching for 'code' tokens...")
    visualizer.search_tokens('code', max_results=50, save_path=f'{output_dir}/token_search_code.html')
    
    print("\n9. Searching for 'func' tokens...")
    visualizer.search_tokens('func', max_results=50, save_path=f'{output_dir}/token_search_func.html')
    
    print("\n10. Generating model representations...")
    visualizer.visualize_layer_outputs(sample_text, save_path=f'{output_dir}/model_representations.html')
    
    print("\n" + "="*60)
    print("All visualizations saved as interactive HTML files!")
    print("="*60)
    print(f"Files created in '{output_dir}/'")
    print("  - attention_all_heads.html")
    print("  - attention_layer0_head0.html")
    print("  - embeddings_tsne_2d.html")
    print("  - embeddings_tsne_3d.html")
    print("  - embeddings_pca_2d.html")
    print("  - embeddings_pca_3d.html")
    print("  - token_similarities.html")
    print("  - token_search_code.html")
    print("  - token_search_func.html")
    print("  - model_representations.html")
    print(f"\nOpen these files from the '{output_dir}/' folder in your browser to interact!")


if __name__ == '__main__':
    main()
