# import os
# os.environ['TF_XLA_FLAGS'] = '--tf_xla_auto_jit=0'
# os.environ['TF_CPP_MIN_LOG_LEVEL'] = '2'

# import tensorflow as tf
# from tensorflow.keras import mixed_precision

# # Enable mixed precision FIRST
# policy = mixed_precision.Policy('mixed_float16')
# mixed_precision.set_global_policy(policy)

# tf.config.optimizer.set_jit(False)
# from tensorflow.keras import layers
# from tensorflow.keras.optimizers import Adafactor
# from tensorflow.keras.layers import Dense, Dropout, LayerNormalization, MultiHeadAttention
# import wandb
# from wandb.integration.keras import WandbMetricsLogger
# from dataGen import DataGenerator


# class CausalBlock(tf.keras.layers.Layer):
#     def __init__(self, d_model, num_heads, ff_dim, dropout_rate=0.1):
#         super().__init__()
#         self.attn = layers.MultiHeadAttention(
#             num_heads=num_heads,
#             key_dim=d_model // num_heads,
#             dropout=dropout_rate
#         )
#         self.ffn = tf.keras.Sequential([
#             layers.Dense(ff_dim, activation='relu'),
#             layers.Dropout(dropout_rate),
#             layers.Dense(d_model),
#             layers.Dropout(dropout_rate),
#         ])
#         self.ln1 = layers.LayerNormalization(epsilon=1e-6)
#         self.ln2 = layers.LayerNormalization(epsilon=1e-6)
#         self.dropout1 = layers.Dropout(dropout_rate)
#         self.dropout2 = layers.Dropout(dropout_rate)

#     def call(self, x, training=False):
#         attn_out = self.attn(x, x, use_causal_mask=True, training=training)
#         attn_out = self.dropout1(attn_out, training=training)
#         x = self.ln1(x + attn_out)
        
#         ffn_out = self.ffn(x, training=training)
#         return self.ln2(x + ffn_out)


# class PositionalEncoding(tf.keras.layers.Layer):
#     def __init__(self, max_len, d_model):
#         super().__init__()
#         self.pos_encoding = self.add_weight(
#             name='pos_encoding',
#             shape=(max_len, d_model),
#             initializer='zeros',
#             trainable=True
#         )
    
#     def call(self, x):
#         seq_len = tf.shape(x)[1]
#         return x + self.pos_encoding[:seq_len, :]


# def build_language_model(vocab_size, context_window, d_model, num_heads, num_layers, ffn_dim=2048, dropout_rate=0.1):
#     inputs = layers.Input(shape=(None,), dtype=tf.int32)
    
#     x = layers.Embedding(input_dim=vocab_size, output_dim=d_model)(inputs)
#     x = layers.Dropout(dropout_rate)(x)
#     x = PositionalEncoding(context_window, d_model)(x)
    
#     for i in range(num_layers):
#         x = CausalBlock(d_model=d_model, num_heads=num_heads, ff_dim=ffn_dim, dropout_rate=dropout_rate)(x)
    
#     x = layers.LayerNormalization(epsilon=1e-6)(x)
    
#     # Cast to float32 for numerical stability with mixed precision
#     x = layers.Lambda(lambda t: tf.cast(t, tf.float32))(x)
#     outputs = layers.Dense(vocab_size, name='output_logits', dtype='float32')(x)
    
#     return tf.keras.Model(inputs=inputs, outputs=outputs, name='causal_language_model')


# if __name__ == '__main__':
#     # OPTIMIZED CONFIGURATION with Mixed Precision
#     CONTEXT_WINDOW = 2048
#     D_MODEL = 1152
#     NUM_HEADS = 18
#     NUM_LAYERS = 38
#     BATCH_SIZE = 1
#     LEARNING_RATE = 0.0001
#     EPOCHS = 1000
#     FFN_DIM = D_MODEL * 4
#     DROPOUT_RATE = 0.1
    
#     wandb.init(
#         project="portfolio-transformer",
#         config={
#             "context_window": CONTEXT_WINDOW,
#             "d_model": D_MODEL,
#             "num_heads": NUM_HEADS,
#             "num_layers": NUM_LAYERS,
#             "batch_size": BATCH_SIZE,
#             "learning_rate": LEARNING_RATE,
#             "epochs": EPOCHS,
#             "ffn_dim": FFN_DIM,
#             "dropout_rate": DROPOUT_RATE,
#             "mixed_precision": True,
#         }
#     )
    
#     data_gen = DataGenerator('extractedTexts.json', 'tokenizer.json')
#     vocab_size = data_gen.vocab_size
#     wandb.config.update({"vocab_size": vocab_size})
    
#     model = build_language_model(
#         vocab_size=vocab_size,
#         context_window=CONTEXT_WINDOW,
#         d_model=D_MODEL,
#         num_heads=NUM_HEADS,
#         num_layers=NUM_LAYERS,
#         ffn_dim=FFN_DIM,
#         dropout_rate=DROPOUT_RATE
#     )
    
#     lr_schedule = tf.keras.optimizers.schedules.CosineDecay(
#         initial_learning_rate=LEARNING_RATE,
#         decay_steps=EPOCHS * (10_000 // BATCH_SIZE),
#         alpha=0.1
#     )
    
#     optimizer = Adafactor(learning_rate=lr_schedule)
    
#     # Wrap optimizer for mixed precision
#     optimizer = mixed_precision.LossScaleOptimizer(optimizer)
    
#     model.compile(
#         optimizer=optimizer,
#         loss=tf.keras.losses.SparseCategoricalCrossentropy(from_logits=True),
#         metrics=[
#             tf.keras.metrics.SparseCategoricalAccuracy(name='accuracy'),
#             tf.keras.metrics.SparseTopKCategoricalAccuracy(k=5, name='top5_accuracy')
#         ]
#     )
    
#     model.build(input_shape=(None, CONTEXT_WINDOW))
#     model.summary()
    
#     print(f"\nTotal parameters: {model.count_params():,}")
#     print(f"Mixed precision enabled: {policy.compute_dtype}")
    
#     bestLoss = float('inf')
#     for epoch in range(EPOCHS):
#         print(f"\n=== Epoch {epoch + 1}/{EPOCHS} ===")
#         steps_per_epoch = 10_000 // BATCH_SIZE
        
#         history = model.fit(
#             data_gen.generate_batches(BATCH_SIZE, CONTEXT_WINDOW),
#             steps_per_epoch=steps_per_epoch,
#             epochs=1,
#             callbacks=[WandbMetricsLogger()],
#             verbose=1
#         )

#         wandb.log({
#             "epoch": epoch + 1,
#             "learning_rate": float(lr_schedule(epoch * steps_per_epoch)),
#             "loss": history.history['loss'][-1],
#             "accuracy": history.history['accuracy'][-1],
#             "top5_accuracy": history.history['top5_accuracy'][-1]
#         })
        
#         current_loss = history.history['loss'][-1]
#         if current_loss < bestLoss:
#             bestLoss = current_loss
#             model.save('best_language_model.keras')
#             print(f"New best model saved with loss: {bestLoss:.4f}")
    
#     wandb.finish()

import os
os.environ['TF_XLA_FLAGS'] = '--tf_xla_auto_jit=0'
os.environ['TF_CPP_MIN_LOG_LEVEL'] = '2'

import tensorflow as tf
from tensorflow.keras import mixed_precision

# Enable mixed precision FIRST
policy = mixed_precision.Policy('mixed_float16')
mixed_precision.set_global_policy(policy)

tf.config.optimizer.set_jit(False)
from tensorflow.keras import layers
from tensorflow.keras.optimizers import Adafactor
import wandb
from wandb.integration.keras import WandbMetricsLogger
from dataGen import DataGenerator


class RMSNorm(layers.Layer):
    """Faster, more stable than LayerNorm - used by LLaMA, Mistral"""
    def __init__(self, dim, eps=1e-6, **kwargs):
        super().__init__(**kwargs)
        self.dim = dim
        self.eps = eps
        
    def build(self, input_shape):
        self.scale = self.add_weight(
            shape=(self.dim,), initializer="ones", trainable=True, name="scale"
        )
        super().build(input_shape)

    def call(self, x):
        rms = tf.sqrt(tf.reduce_mean(tf.square(x), axis=-1, keepdims=True) + self.eps)
        return x / rms * self.scale
    
    def get_config(self):
        config = super().get_config()
        config.update({
            "dim": self.dim,
            "eps": self.eps,
        })
        return config


class SwiGLU(layers.Layer):
    """Gated activation - major improvement over ReLU FFN"""
    def __init__(self, d_model, hidden_dim, dropout_rate=0.1, **kwargs):
        super().__init__(**kwargs)
        self.d_model = d_model
        self.hidden_dim = hidden_dim
        self.dropout_rate = dropout_rate
        
    def build(self, input_shape):
        self.w1 = layers.Dense(self.hidden_dim, use_bias=False)
        self.w2 = layers.Dense(self.hidden_dim, use_bias=False)
        self.w3 = layers.Dense(self.d_model, use_bias=False)
        self.dropout = layers.Dropout(self.dropout_rate)
        super().build(input_shape)

    def call(self, x, training=False):
        gate = tf.nn.silu(self.w1(x))
        x = gate * self.w2(x)
        x = self.dropout(x, training=training)
        return self.w3(x)
    
    def get_config(self):
        config = super().get_config()
        config.update({
            "d_model": self.d_model,
            "hidden_dim": self.hidden_dim,
            "dropout_rate": self.dropout_rate,
        })
        return config


class CausalBlock(tf.keras.layers.Layer):
    def __init__(self, d_model, num_heads, ff_dim, dropout_rate=0.1, layer_idx=0, num_layers=1, **kwargs):
        super().__init__(**kwargs)
        self.d_model = d_model
        self.num_heads = num_heads
        self.ff_dim = ff_dim
        self.dropout_rate = dropout_rate
        self.layer_idx = layer_idx
        self.num_layers = num_layers
        
    def build(self, input_shape):
        self.attn = layers.MultiHeadAttention(
            num_heads=self.num_heads,
            key_dim=self.d_model // self.num_heads,
            dropout=self.dropout_rate
        )
        
        # SwiGLU instead of ReLU FFN
        swiglu_dim = int(self.ff_dim * 2 / 3)  # Standard practice
        self.ffn = SwiGLU(self.d_model, swiglu_dim, self.dropout_rate)
        
        # RMSNorm instead of LayerNorm
        self.rms1 = RMSNorm(self.d_model)
        self.rms2 = RMSNorm(self.d_model)
        
        self.dropout1 = layers.Dropout(self.dropout_rate)
        super().build(input_shape)

    def call(self, x, training=False):
        # Pre-norm + scaled residual for stability
        residual = x
        x = self.rms1(x)
        attn_out = self.attn(x, x, use_causal_mask=True, training=training)
        attn_out = self.dropout1(attn_out, training=training)
        
        # Scaled residual connection (muParam-style)
        scale = tf.cast(tf.sqrt(float(self.num_layers)), x.dtype)
        x = residual + attn_out / scale
        
        # FFN block
        residual = x
        x = self.rms2(x)
        ffn_out = self.ffn(x, training=training)
        x = residual + ffn_out / scale
        
        return x
    
    def get_config(self):
        config = super().get_config()
        config.update({
            "d_model": self.d_model,
            "num_heads": self.num_heads,
            "ff_dim": self.ff_dim,
            "dropout_rate": self.dropout_rate,
            "layer_idx": self.layer_idx,
            "num_layers": self.num_layers,
        })
        return config


class PositionalEncoding(tf.keras.layers.Layer):
    def __init__(self, max_len, d_model, **kwargs):
        super().__init__(**kwargs)
        self.max_len = max_len
        self.d_model = d_model
        
    def build(self, input_shape):
        self.pos_encoding = self.add_weight(
            name='pos_encoding',
            shape=(self.max_len, self.d_model),
            initializer='zeros',
            trainable=True
        )
        super().build(input_shape)
    
    def call(self, x):
        seq_len = tf.shape(x)[1]
        return x + self.pos_encoding[:seq_len, :]
    
    def get_config(self):
        config = super().get_config()
        config.update({
            "max_len": self.max_len,
            "d_model": self.d_model,
        })
        return config


class TiedOutputLayer(layers.Layer):
    """Output layer that shares weights with embedding layer"""
    def __init__(self, vocab_size, d_model, **kwargs):
        super().__init__(**kwargs)
        self.vocab_size = vocab_size
        self.d_model = d_model
        self._embedding_layer = None
    
    def set_embedding_layer(self, embedding_layer):
        """Call this after initialization to set the embedding layer reference"""
        self._embedding_layer = embedding_layer
    
    def call(self, x):
        # Cast x to float32 for stable computation
        x = tf.cast(x, tf.float32)
        embeddings = tf.cast(self._embedding_layer.embeddings, tf.float32)
        return tf.matmul(x, embeddings, transpose_b=True)
    
    def get_config(self):
        config = super().get_config()
        config.update({
            "vocab_size": self.vocab_size,
            "d_model": self.d_model,
        })
        return config
    
    @classmethod
    def from_config(cls, config):
        return cls(**config)


def build_language_model(vocab_size, context_window, d_model, num_heads, num_layers, ffn_dim=2048, dropout_rate=0.1):
    inputs = layers.Input(shape=(None,), dtype=tf.int32)
    
    # Embedding layer (will be tied to output)
    embedding_layer = layers.Embedding(input_dim=vocab_size, output_dim=d_model, name='token_embedding')
    x = embedding_layer(inputs)
    x = layers.Dropout(dropout_rate)(x)
    x = PositionalEncoding(context_window, d_model)(x)
    
    # Transformer blocks
    for i in range(num_layers):
        x = CausalBlock(
            d_model=d_model, 
            num_heads=num_heads, 
            ff_dim=ffn_dim, 
            dropout_rate=dropout_rate,
            layer_idx=i,
            num_layers=num_layers
        )(x)
    
    # Final norm
    x = RMSNorm(d_model)(x)
    
    # Output layer with weight tying
    output_layer = TiedOutputLayer(vocab_size, d_model, name='output_logits')
    output_layer.set_embedding_layer(embedding_layer)
    outputs = output_layer(x)
    
    model = tf.keras.Model(inputs=inputs, outputs=outputs, name='causal_language_model')
    
    return model

LOAD_BEST_MODEL = True

if __name__ == '__main__':
    # OPTIMIZED CONFIGURATION
    CONTEXT_WINDOW = 2048
    D_MODEL = 1152
    NUM_HEADS = 18
    NUM_LAYERS = 44
    BATCH_SIZE = 1
    LEARNING_RATE = 0.0001
    EPOCHS = 10_000
    FFN_DIM = D_MODEL * 4
    DROPOUT_RATE = 0.1
    WEIGHTS_PATH = 'best_model.weights.h5'
    
    wandb.init(
        project="portfolio-transformer",
        config={
            "context_window": CONTEXT_WINDOW,
            "d_model": D_MODEL,
            "num_heads": NUM_HEADS,
            "num_layers": NUM_LAYERS,
            "batch_size": BATCH_SIZE,
            "learning_rate": LEARNING_RATE,
            "epochs": EPOCHS,
            "ffn_dim": FFN_DIM,
            "dropout_rate": DROPOUT_RATE,
            "mixed_precision": True,
            "improvements": "RMSNorm + SwiGLU + WeightTying + ScaledResiduals",
            "load_best_model": LOAD_BEST_MODEL
        }
    )
    
    # data_gen = DataGenerator('extractedTexts.json', 'tokenizer.json')
    # data_gen = DataGenerator('extractedTexts_multilang.json', 'tokenizer.json')
    data_gen = DataGenerator(
        [
            'extractedTexts_multilang.json',
            # 'extractedTexts.json',
            'extractedTexts_OpenThoughts.json',
          ],
        'tokenizer.json',
    )
    data_gen.preload_all_files()
    vocab_size = data_gen.vocab_size
    wandb.config.update({"vocab_size": vocab_size})
    
    # Always build the model architecture
    model = build_language_model(
        vocab_size=vocab_size,
        context_window=CONTEXT_WINDOW,
        d_model=D_MODEL,
        num_heads=NUM_HEADS,
        num_layers=NUM_LAYERS,
        ffn_dim=FFN_DIM,
        dropout_rate=DROPOUT_RATE
    )
    
    model.build(input_shape=(None, CONTEXT_WINDOW))
    
    # Load weights if available
    if LOAD_BEST_MODEL and os.path.exists(WEIGHTS_PATH):
        print(f"\n{'='*60}")
        print(f"LOADING MODEL WEIGHTS from {WEIGHTS_PATH}")
        print(f"{'='*60}\n")
        
        model.load_weights(WEIGHTS_PATH)
        
        print("Weights loaded successfully!")
        print(f"Total parameters: {model.count_params():,}\n")
    else:
        if LOAD_BEST_MODEL:
            print(f"\nWeights file not found at {WEIGHTS_PATH}")
            print("Starting with fresh model initialization...\n")
        
        model.summary()
        
        print(f"\nMODERNIZED MODEL")
        print(f"Total parameters: {model.count_params():,}")
        print(f"Improvements: RMSNorm, SwiGLU, Weight Tying, Scaled Residuals")
        print(f"Mixed precision: {policy.compute_dtype}")
    
    lr_schedule = tf.keras.optimizers.schedules.CosineDecay(
        initial_learning_rate=LEARNING_RATE,
        decay_steps=EPOCHS * (1_000 // BATCH_SIZE),
        alpha=0.1
    )
    
    optimizer = Adafactor(learning_rate=lr_schedule)
    optimizer = mixed_precision.LossScaleOptimizer(optimizer)
    
    model.compile(
        optimizer=optimizer,
        loss=tf.keras.losses.SparseCategoricalCrossentropy(from_logits=True),
        metrics=[
            tf.keras.metrics.SparseCategoricalAccuracy(name='accuracy'),
            tf.keras.metrics.SparseTopKCategoricalAccuracy(k=5, name='top5_accuracy')
        ]
    )
    
    bestLoss = float('inf')
    for epoch in range(EPOCHS):
        print(f"\n=== Epoch {epoch + 1}/{EPOCHS} ===")
        steps_per_epoch = 1_000 // BATCH_SIZE
        
        history = model.fit(
            data_gen.generate_batches(BATCH_SIZE, CONTEXT_WINDOW),
            steps_per_epoch=steps_per_epoch,
            epochs=1,
            callbacks=[WandbMetricsLogger()],
            verbose=1
        )

        wandb.log({
            "epoch": epoch + 1,
            "learning_rate": float(lr_schedule(epoch * steps_per_epoch)),
            "loss": history.history['loss'][-1],
            "accuracy": history.history['accuracy'][-1],
            "top5_accuracy": history.history['top5_accuracy'][-1]
        })
        
        current_loss = history.history['loss'][-1]
        if current_loss < bestLoss:
            bestLoss = current_loss
            model.save_weights(WEIGHTS_PATH)
            print(f"New best model saved with loss: {bestLoss:.4f}")
    
    wandb.finish()