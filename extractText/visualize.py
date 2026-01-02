import tensorflow as tf
from tensorflow.keras import mixed_precision
import numpy as np
from tokenizers import Tokenizer
from train import build_language_model
import plotly.graph_objects as go
import plotly.express as px
from plotly.subplots import make_subplots
from sklearn.manifold import TSNE
from sklearn.decomposition import PCA

policy = mixed_precision.Policy('mixed_float16')
mixed_precision.set_global_policy(policy)


class ModelVisualizer:
    def __init__(self, weights_path, tokenizer_path, context_window=2048, d_model=1152, num_heads=18, num_layers=44, dropout_rate=0.1):
        self.tokenizer = Tokenizer.from_file(tokenizer_path)
        self.vocab_size = self.tokenizer.get_vocab_size()
        self.num_heads = num_heads
        self.num_layers = num_layers
        
        print(f"Building model architecture...")
        self.model = build_language_model(
            vocab_size=self.vocab_size,
            context_window=context_window,
            d_model=d_model,
            num_heads=num_heads,
            num_layers=num_layers,
            ffn_dim=d_model * 4,
            dropout_rate=dropout_rate
        )
        
        self.model.build(input_shape=(None, context_window))
        
        print(f"Loading weights from {weights_path}...")
        self.model.load_weights(weights_path)
        print(f"Model loaded! Total parameters: {self.model.count_params():,}\n")
        
        self.embedding_layer = None
        for layer in self.model.layers:
            if 'token_embedding' in layer.name:
                self.embedding_layer = layer
                break
    
    def create_attention_model(self, layer_idx):
        inputs = self.model.input
        
        causal_blocks = [layer for layer in self.model.layers if 'causal_block' in layer.name.lower()]
        
        if layer_idx >= len(causal_blocks):
            raise ValueError(f"Layer {layer_idx} not found. Model has {len(causal_blocks)} causal blocks.")
        
        target_block = causal_blocks[layer_idx]
        
        attention_layer = target_block.attn
        
        intermediate_model = tf.keras.Model(
            inputs=inputs,
            outputs=target_block.output
        )
        
        return intermediate_model, attention_layer, target_block
    
    def extract_attention_weights(self, text, layer_idx=0):
        tokens = self.tokenizer.encode(text).ids
        input_seq = np.array([tokens], dtype=np.int32)
        
        intermediate_model, attention_layer, block = self.create_attention_model(layer_idx)
        
        class AttentionExtractor(tf.keras.Model):
            def __init__(self, base_model, attention_layer):
                super().__init__()
                self.base_model = base_model
                self.attention_layer = attention_layer
                
            def call(self, inputs):
                with tf.GradientTape() as tape:
                    x = inputs
                    for i, layer in enumerate(self.base_model.layers):
                        if layer == block:
                            rms_norm = block.rms1(x)
                            attention_output = self.attention_layer(
                                rms_norm, rms_norm,
                                use_causal_mask=True,
                                return_attention_scores=True
                            )
                            return attention_output
                        x = layer(x)
        
        extractor = AttentionExtractor(self.model, attention_layer)
        
        try:
            result = attention_layer(
                block.rms1(self.model.layers[2](self.embedding_layer(input_seq))),
                block.rms1(self.model.layers[2](self.embedding_layer(input_seq))),
                use_causal_mask=True,
                return_attention_scores=True
            )
            
            if isinstance(result, tuple):
                _, attention_scores = result
            else:
                attention_scores = result
                
            return attention_scores.numpy(), tokens
        except Exception as e:
            print(f"Could not extract attention: {e}")
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
                xaxis_title='Keys (What tokens are attended TO)',
                yaxis_title='Queries (What tokens are attending)',
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
                        showscale=(head == num_heads - 1),
                        hoverongaps=False,
                        colorbar=dict(title="Attention<br>Weight") if head == num_heads - 1 else None
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
            
            for i in range(1, rows * cols + 1):
                row_idx = (i-1)//cols + 1
                col_idx = (i-1)%cols + 1
                fig.update_xaxes(
                    title_text='Keys (Attended TO)' if row_idx == rows else None,
                    tickangle=45,
                    row=row_idx, 
                    col=col_idx
                )
                fig.update_yaxes(
                    title_text='Queries (Attending)' if col_idx == 1 else None,
                    row=row_idx, 
                    col=col_idx
                )
        
        if save_path:
            fig.write_html(save_path)
            print(f"Saved to {save_path}")
        else:
            fig.show()
    
    def visualize_embeddings(self, tokens, method='tsne', sample_size=1000, save_path=None, use_3d=False):
        if self.embedding_layer is None:
            print("Embedding layer not found!")
            return
        
        embeddings = self.embedding_layer.embeddings.numpy()
        
        if len(embeddings) > sample_size:
            indices = np.random.choice(len(embeddings), sample_size, replace=False)
            if isinstance(tokens, list):
                indices = np.concatenate([indices, tokens])
                indices = np.unique(indices)
            embeddings_sample = embeddings[indices]
        else:
            embeddings_sample = embeddings
            indices = np.arange(len(embeddings))
        
        n_components = 3 if use_3d else 2
        
        if method == 'tsne':
            reducer = TSNE(n_components=n_components, random_state=42, perplexity=30)
            print(f"Computing {n_components}D t-SNE projection...")
        else:
            reducer = PCA(n_components=n_components, random_state=42)
            print(f"Computing {n_components}D PCA projection...")
        
        embeddings_reduced = reducer.fit_transform(embeddings_sample)
        
        labels = [self.tokenizer.decode([int(i)]) for i in indices]
        
        highlight = []
        if isinstance(tokens, list):
            highlight = [i in tokens for i in indices]
        
        fig = go.Figure()
        
        if use_3d:
            if any(highlight):
                fig.add_trace(go.Scatter3d(
                    x=embeddings_reduced[~np.array(highlight), 0],
                    y=embeddings_reduced[~np.array(highlight), 1],
                    z=embeddings_reduced[~np.array(highlight), 2],
                    mode='markers',
                    marker=dict(size=3, color='lightblue', opacity=0.6),
                    text=[labels[i] for i in range(len(labels)) if not highlight[i]],
                    name='Other tokens',
                    hovertemplate='<b>%{text}</b><br>X: %{x:.2f}<br>Y: %{y:.2f}<br>Z: %{z:.2f}<extra></extra>'
                ))
                
                fig.add_trace(go.Scatter3d(
                    x=embeddings_reduced[highlight, 0],
                    y=embeddings_reduced[highlight, 1],
                    z=embeddings_reduced[highlight, 2],
                    mode='markers+text',
                    marker=dict(size=8, color='red', opacity=0.9),
                    text=[labels[i] for i in range(len(labels)) if highlight[i]],
                    textposition='top center',
                    name='Input tokens',
                    hovertemplate='<b>%{text}</b><br>X: %{x:.2f}<br>Y: %{y:.2f}<br>Z: %{z:.2f}<extra></extra>'
                ))
            else:
                fig.add_trace(go.Scatter3d(
                    x=embeddings_reduced[:, 0],
                    y=embeddings_reduced[:, 1],
                    z=embeddings_reduced[:, 2],
                    mode='markers',
                    marker=dict(size=3, color='blue', opacity=0.6),
                    text=labels,
                    hovertemplate='<b>%{text}</b><br>X: %{x:.2f}<br>Y: %{y:.2f}<br>Z: %{z:.2f}<extra></extra>'
                ))
            
            fig.update_layout(
                title=dict(
                    text=f'3D Token Embedding Space - {method.upper()} Projection<br><sub>Visualizing {len(indices)} tokens in 3D space - Rotate and zoom to explore</sub>',
                    x=0.5,
                    xanchor='center'
                ),
                scene=dict(
                    xaxis_title=f'{method.upper()} 1',
                    yaxis_title=f'{method.upper()} 2',
                    zaxis_title=f'{method.upper()} 3',
                    camera=dict(
                        eye=dict(x=1.5, y=1.5, z=1.5)
                    )
                ),
                width=1200,
                height=900,
                hovermode='closest',
                font=dict(size=12)
            )
        else:
            if any(highlight):
                fig.add_trace(go.Scatter(
                    x=embeddings_reduced[~np.array(highlight), 0],
                    y=embeddings_reduced[~np.array(highlight), 1],
                    mode='markers',
                    marker=dict(size=5, color='lightblue', opacity=0.6),
                    text=[labels[i] for i in range(len(labels)) if not highlight[i]],
                    name='Other tokens',
                    hovertemplate='<b>%{text}</b><br>X: %{x:.2f}<br>Y: %{y:.2f}<extra></extra>'
                ))
                
                fig.add_trace(go.Scatter(
                    x=embeddings_reduced[highlight, 0],
                    y=embeddings_reduced[highlight, 1],
                    mode='markers+text',
                    marker=dict(size=10, color='red', opacity=0.9),
                    text=[labels[i] for i in range(len(labels)) if highlight[i]],
                    textposition='top center',
                    name='Input tokens',
                    hovertemplate='<b>%{text}</b><br>X: %{x:.2f}<br>Y: %{y:.2f}<extra></extra>'
                ))
            else:
                fig.add_trace(go.Scatter(
                    x=embeddings_reduced[:, 0],
                    y=embeddings_reduced[:, 1],
                    mode='markers',
                    marker=dict(size=5, color='blue', opacity=0.6),
                    text=labels,
                    hovertemplate='<b>%{text}</b><br>X: %{x:.2f}<br>Y: %{y:.2f}<extra></extra>'
                ))
            
            fig.update_layout(
                title=dict(
                    text=f'Token Embedding Space - {method.upper()} Projection<br><sub>Visualizing {len(indices)} tokens in 2D space</sub>',
                    x=0.5,
                    xanchor='center'
                ),
                xaxis_title=f'{method.upper()} Component 1',
                yaxis_title=f'{method.upper()} Component 2',
                width=1100,
                height=900,
                hovermode='closest',
                font=dict(size=12),
                plot_bgcolor='rgba(240,240,240,0.5)'
            )
        
        if save_path:
            fig.write_html(save_path)
            print(f"Saved to {save_path}")
        else:
            fig.show()
    
    def visualize_token_similarities(self, tokens, top_k=10, save_path=None):
        if self.embedding_layer is None:
            print("Embedding layer not found!")
            return
        
        embeddings = self.embedding_layer.embeddings.numpy()
        
        if isinstance(tokens, str):
            tokens = self.tokenizer.encode(tokens).ids
        
        token_labels = [self.tokenizer.decode([t]) for t in tokens]
        
        similarity_matrix = []
        similar_token_labels = []
        
        for token in tokens:
            token_emb = embeddings[token]
            
            similarities = np.dot(embeddings, token_emb) / (
                np.linalg.norm(embeddings, axis=1) * np.linalg.norm(token_emb)
            )
            
            top_indices = np.argsort(similarities)[-top_k:][::-1]
            similarity_matrix.append([similarities[idx] for idx in top_indices])
            
            if not similar_token_labels:
                similar_token_labels = [self.tokenizer.decode([idx]) for idx in top_indices]
        
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
        if self.embedding_layer is None:
            print("Embedding layer not found!")
            return
        
        print(f"Searching for tokens matching: '{search_query}'...")
        
        matching_indices = []
        matching_labels = []
        
        for i in range(self.vocab_size):
            token_text = self.tokenizer.decode([i])
            if search_query.lower() in token_text.lower():
                matching_indices.append(i)
                matching_labels.append(token_text)
                if len(matching_indices) >= max_results:
                    break
        
        if not matching_indices:
            print(f"No tokens found matching '{search_query}'")
            return
        
        print(f"Found {len(matching_indices)} matching tokens")
        
        embeddings = self.embedding_layer.embeddings.numpy()
        matching_embeddings = embeddings[matching_indices]
        
        use_3d = len(matching_indices) >= 3
        n_components = 3 if use_3d else min(2, len(matching_indices))
        
        if len(matching_indices) < 3:
            print(f"Note: Using {n_components}D visualization (need at least 3 tokens for 3D)")
        
        reducer = PCA(n_components=n_components, random_state=42)
        embeddings_reduced = reducer.fit_transform(matching_embeddings)
        
        if use_3d:
            fig = go.Figure(data=[go.Scatter3d(
                x=embeddings_reduced[:, 0],
                y=embeddings_reduced[:, 1],
                z=embeddings_reduced[:, 2],
                mode='markers+text',
                marker=dict(
                    size=6,
                    color=np.arange(len(matching_indices)),
                    colorscale='Viridis',
                    showscale=True,
                    colorbar=dict(title="Token<br>Index")
                ),
                text=matching_labels,
                textposition='top center',
                textfont=dict(size=10),
                hovertemplate='<b>%{text}</b><br>Token ID: ' + 
                             str(matching_indices) + '<br>X: %{x:.2f}<br>Y: %{y:.2f}<br>Z: %{z:.2f}<extra></extra>'
            )])
            
            fig.update_layout(
                title=dict(
                    text=f'Token Search Results: "{search_query}"<br><sub>Found {len(matching_indices)} matching tokens in 3D embedding space</sub>',
                    x=0.5,
                    xanchor='center'
                ),
                scene=dict(
                    xaxis_title='PCA Component 1',
                    yaxis_title='PCA Component 2',
                    zaxis_title='PCA Component 3',
                    camera=dict(eye=dict(x=1.5, y=1.5, z=1.5))
                ),
                width=1300,
                height=900,
                font=dict(size=12)
            )
        else:
            fig = go.Figure(data=[go.Scatter(
                x=embeddings_reduced[:, 0],
                y=embeddings_reduced[:, 1] if n_components == 2 else [0] * len(matching_indices),
                mode='markers+text',
                marker=dict(
                    size=10,
                    color=np.arange(len(matching_indices)),
                    colorscale='Viridis',
                    showscale=True,
                    colorbar=dict(title="Token<br>Index")
                ),
                text=matching_labels,
                textposition='top center',
                textfont=dict(size=12),
                hovertemplate='<b>%{text}</b><br>Token ID: ' + 
                             str(matching_indices) + '<br>X: %{x:.2f}<br>Y: %{y:.2f}<extra></extra>'
            )])
            
            fig.update_layout(
                title=dict(
                    text=f'Token Search Results: "{search_query}"<br><sub>Found {len(matching_indices)} matching tokens in {n_components}D embedding space</sub>',
                    x=0.5,
                    xanchor='center'
                ),
                xaxis_title='PCA Component 1',
                yaxis_title='PCA Component 2' if n_components == 2 else '',
                width=1300,
                height=900,
                font=dict(size=12),
                plot_bgcolor='rgba(240,240,240,0.5)'
            )
        
        if save_path:
            fig.write_html(save_path)
            print(f"Saved to {save_path}")
        else:
            fig.show()
    
    def visualize_layer_outputs(self, text, save_path=None):
        tokens = self.tokenizer.encode(text).ids
        input_seq = np.array([tokens], dtype=np.int32)
        
        token_labels = [self.tokenizer.decode([t]) for t in tokens]
        
        embedding_output = self.embedding_layer(input_seq).numpy()
        
        predictions = self.model.predict(input_seq, verbose=0)
        
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
    
    def visualize_loss_landscape(self, data_generator, num_samples=10, grid_size=20, alpha_range=1.0, save_path=None):
        from dataGen import DataGenerator
        
        print("Computing loss landscape (this may take a while)...")
        
        original_weights = [w.numpy() for w in self.model.trainable_weights]
        
        print("Generating random perturbation directions...")
        direction1 = [np.random.randn(*w.shape).astype(np.float32) for w in original_weights]
        direction2 = [np.random.randn(*w.shape).astype(np.float32) for w in original_weights]
        
        for i in range(len(direction1)):
            norm1 = np.linalg.norm(direction1[i])
            norm2 = np.linalg.norm(direction2[i])
            if norm1 > 0:
                direction1[i] = direction1[i] / norm1
            if norm2 > 0:
                direction2[i] = direction2[i] / norm2
        
        print(f"Preparing {num_samples} samples for loss evaluation...")
        sample_data = []
        for X_batch, Y_batch in data_generator.generate_batches(batch_size=1, max_sample_length=512):
            sample_data.append((X_batch, Y_batch))
            if len(sample_data) >= num_samples:
                break
        
        print(f"Computing loss at {grid_size}x{grid_size} = {grid_size*grid_size} points...")
        alphas = np.linspace(-alpha_range, alpha_range, grid_size)
        betas = np.linspace(-alpha_range, alpha_range, grid_size)
        
        loss_grid = np.zeros((grid_size, grid_size))
        
        loss_fn = tf.keras.losses.SparseCategoricalCrossentropy(from_logits=True)
        
        total_points = grid_size * grid_size
        completed = 0
        
        for i, alpha in enumerate(alphas):
            for j, beta in enumerate(betas):
                new_weights = [
                    w + alpha * d1 + beta * d2 
                    for w, d1, d2 in zip(original_weights, direction1, direction2)
                ]
                
                for k, weight in enumerate(self.model.trainable_weights):
                    weight.assign(new_weights[k])
                
                total_loss = 0.0
                for X_batch, Y_batch in sample_data:
                    predictions = self.model(X_batch, training=False)
                    loss = loss_fn(Y_batch, predictions)
                    total_loss += loss.numpy()
                
                loss_grid[i, j] = total_loss / len(sample_data)
                
                completed += 1
                if completed % max(1, total_points // 20) == 0:
                    progress = (completed / total_points) * 100
                    print(f"Progress: {progress:.1f}% ({completed}/{total_points} points)")
        
        for k, weight in enumerate(self.model.trainable_weights):
            weight.assign(original_weights[k])
        
        print("Creating 3D visualization...")
        
        fig = go.Figure(data=[go.Surface(
            x=betas,
            y=alphas,
            z=loss_grid,
            colorscale='Viridis',
            colorbar=dict(title="Loss"),
            hovertemplate='α: %{y:.3f}<br>β: %{x:.3f}<br>Loss: %{z:.4f}<extra></extra>'
        )])
        
        origin_idx_alpha = grid_size // 2
        origin_idx_beta = grid_size // 2
        origin_loss = loss_grid[origin_idx_alpha, origin_idx_beta]
        
        fig.add_trace(go.Scatter3d(
            x=[0],
            y=[0],
            z=[origin_loss],
            mode='markers',
            marker=dict(size=10, color='red', symbol='diamond'),
            name='Current Model',
            hovertemplate='Current Model<br>Loss: %{z:.4f}<extra></extra>'
        ))
        
        fig.update_layout(
            title=dict(
                text=f'3D Loss Landscape<br><sub>Random 2D slice through weight space ({grid_size}x{grid_size} grid, {num_samples} samples)</sub>',
                x=0.5,
                xanchor='center'
            ),
            scene=dict(
                xaxis_title='Perturbation β',
                yaxis_title='Perturbation α',
                zaxis_title='Loss',
                camera=dict(
                    eye=dict(x=1.5, y=1.5, z=1.2)
                )
            ),
            width=1200,
            height=900,
            font=dict(size=12)
        )
        
        print(f"Loss at current model: {origin_loss:.4f}")
        print(f"Min loss in landscape: {np.min(loss_grid):.4f}")
        print(f"Max loss in landscape: {np.max(loss_grid):.4f}")
        
        if save_path:
            fig.write_html(save_path)
            print(f"Saved to {save_path}")
        else:
            fig.show()


def main():
    import os
    from dataGen import DataGenerator
    
    output_dir = 'visualizations'
    os.makedirs(output_dir, exist_ok=True)
    
    print("Initializing visualizer...")
    visualizer = ModelVisualizer(
        weights_path='best_model.weights.h5',
        tokenizer_path='tokenizer.json',
        context_window=2048,
        d_model=1152,
        num_heads=18,
        num_layers=44,
        dropout_rate=0.1
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
    
    print(f"\n3. Generating 2D token embeddings (t-SNE) - ALL {visualizer.vocab_size} TOKENS...")
    tokens = visualizer.tokenizer.encode(sample_text).ids
    visualizer.visualize_embeddings(tokens, method='tsne', sample_size=visualizer.vocab_size, save_path=f'{output_dir}/embeddings_tsne_2d.html', use_3d=False)
    
    print(f"\n4. Generating 3D token embeddings (t-SNE) - ALL {visualizer.vocab_size} TOKENS...")
    visualizer.visualize_embeddings(tokens, method='tsne', sample_size=visualizer.vocab_size, save_path=f'{output_dir}/embeddings_tsne_3d.html', use_3d=True)
    
    print(f"\n5. Generating 2D token embeddings (PCA) - ALL {visualizer.vocab_size} TOKENS...")
    visualizer.visualize_embeddings(tokens, method='pca', sample_size=visualizer.vocab_size, save_path=f'{output_dir}/embeddings_pca_2d.html', use_3d=False)
    
    print(f"\n6. Generating 3D token embeddings (PCA) - ALL {visualizer.vocab_size} TOKENS...")
    visualizer.visualize_embeddings(tokens, method='pca', sample_size=visualizer.vocab_size, save_path=f'{output_dir}/embeddings_pca_3d.html', use_3d=True)
    
    print("\n7. Generating token similarities...")
    visualizer.visualize_token_similarities(sample_text, top_k=10, save_path=f'{output_dir}/token_similarities.html')
    
    print("\n8. Searching for 'python' tokens...")
    visualizer.search_tokens('python', max_results=50, save_path=f'{output_dir}/token_search_python.html')
    
    print("\n9. Searching for 'func' tokens...")
    visualizer.search_tokens('func', max_results=50, save_path=f'{output_dir}/token_search_func.html')
    
    print("\n10. Generating model representations...")
    visualizer.visualize_layer_outputs(sample_text, save_path=f'{output_dir}/model_representations.html')
    
    print("\n11. Generating 3D loss landscape...")
    print("Loading data generator for loss landscape...")
    data_gen = DataGenerator(
        ['extractedTexts.json'],
        'tokenizer.json',
        cache_size=1,
        lazy_count=True
    )
    visualizer.visualize_loss_landscape(
        data_gen, 
        num_samples=2000, 
        grid_size=24, 
        alpha_range=0.5,
        save_path=f'{output_dir}/loss_landscape_3d.html'
    )
    
    print("\n" + "="*60)
    print("All visualizations saved as interactive HTML files!")
    print("="*60)
    print(f"Files created in '{output_dir}/'")
    print("  - attention_all_heads.html (Interactive multi-head attention)")
    print("  - attention_layer0_head0.html (Single attention head)")
    print(f"  - embeddings_tsne_2d.html (2D t-SNE - {visualizer.vocab_size} tokens)")
    print(f"  - embeddings_tsne_3d.html (3D t-SNE - {visualizer.vocab_size} tokens, rotatable!)")
    print(f"  - embeddings_pca_2d.html (2D PCA - {visualizer.vocab_size} tokens)")
    print(f"  - embeddings_pca_3d.html (3D PCA - {visualizer.vocab_size} tokens, rotatable!)")
    print("  - token_similarities.html (Similarity analysis)")
    print("  - token_search_python.html (Tokens containing 'python')")
    print("  - token_search_func.html (Tokens containing 'func')")
    print("  - model_representations.html (Embeddings & output logits)")
    print(f"\nOpen these files from the '{output_dir}/' folder in your browser to interact!")
    print("Tip: 3D visualizations can be rotated, zoomed, and panned for exploration.")


if __name__ == '__main__':
    main()
