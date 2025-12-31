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
    def __init__(self, dim, eps=1e-6):
        super().__init__()
        self.eps = eps
        self.scale = self.add_weight(
            shape=(dim,), initializer="ones", trainable=True, name="scale"
        )

    def call(self, x):
        rms = tf.sqrt(tf.reduce_mean(tf.square(x), axis=-1, keepdims=True) + self.eps)
        return x / rms * self.scale


class SwiGLU(layers.Layer):
    """Gated activation - major improvement over ReLU FFN"""
    def __init__(self, d_model, hidden_dim, dropout_rate=0.1):
        super().__init__()
        self.w1 = layers.Dense(hidden_dim, use_bias=False)
        self.w2 = layers.Dense(hidden_dim, use_bias=False)
        self.w3 = layers.Dense(d_model, use_bias=False)
        self.dropout = layers.Dropout(dropout_rate)

    def call(self, x, training=False):
        gate = tf.nn.silu(self.w1(x))
        x = gate * self.w2(x)
        x = self.dropout(x, training=training)
        return self.w3(x)


class CausalBlock(tf.keras.layers.Layer):
    def __init__(self, d_model, num_heads, ff_dim, dropout_rate=0.1, layer_idx=0, num_layers=1):
        super().__init__()
        self.layer_idx = layer_idx
        self.num_layers = num_layers
        
        self.attn = layers.MultiHeadAttention(
            num_heads=num_heads,
            key_dim=d_model // num_heads,
            dropout=dropout_rate
        )
        
        # SwiGLU instead of ReLU FFN
        swiglu_dim = int(ff_dim * 2 / 3)  # Standard practice
        self.ffn = SwiGLU(d_model, swiglu_dim, dropout_rate)
        
        # RMSNorm instead of LayerNorm
        self.rms1 = RMSNorm(d_model)
        self.rms2 = RMSNorm(d_model)
        
        self.dropout1 = layers.Dropout(dropout_rate)

    def call(self, x, training=False):
        # Pre-norm + scaled residual for stability
        residual = x
        x = self.rms1(x)
        attn_out = self.attn(x, x, use_causal_mask=True, training=training)
        attn_out = self.dropout1(attn_out, training=training)
        
        # Scaled residual connection (μParam-style)
        scale = tf.sqrt(float(self.num_layers))
        x = residual + attn_out / scale
        
        # FFN block
        residual = x
        x = self.rms2(x)
        ffn_out = self.ffn(x, training=training)
        x = residual + ffn_out / scale
        
        return x


class PositionalEncoding(tf.keras.layers.Layer):
    def __init__(self, max_len, d_model):
        super().__init__()
        self.pos_encoding = self.add_weight(
            name='pos_encoding',
            shape=(max_len, d_model),
            initializer='zeros',
            trainable=True
        )
    
    def call(self, x):
        seq_len = tf.shape(x)[1]
        return x + self.pos_encoding[:seq_len, :]


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
    
    # Cast to float32 for numerical stability
    x = layers.Lambda(lambda t: tf.cast(t, tf.float32))(x)
    
    # Output layer with weight tying
    output_dense = layers.Dense(vocab_size, use_bias=False, name='output_logits', dtype='float32')
    outputs = output_dense(x)
    
    model = tf.keras.Model(inputs=inputs, outputs=outputs, name='causal_language_model')
    
    # WEIGHT TYING: Share embedding weights with output projection
    # This reduces parameters and improves training
    output_dense.kernel = tf.Variable(
        tf.transpose(embedding_layer.embeddings),
        trainable=True,
        name='tied_output_kernel'
    )
    
    return model


if __name__ == '__main__':
    CONTEXT_WINDOW = 2048
    D_MODEL = 1152
    NUM_HEADS = 18
    NUM_LAYERS = 38
    BATCH_SIZE = 1
    LEARNING_RATE = 0.0001
    EPOCHS = 1000
    FFN_DIM = D_MODEL * 4
    DROPOUT_RATE = 0.1
    
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
            "improvements": "RMSNorm + SwiGLU + WeightTying + ScaledResiduals"
        }
    )
    
    data_gen = DataGenerator('extractedTexts.json', 'tokenizer.json')
    vocab_size = data_gen.vocab_size
    wandb.config.update({"vocab_size": vocab_size})
    
    model = build_language_model(
        vocab_size=vocab_size,
        context_window=CONTEXT_WINDOW,
        d_model=D_MODEL,
        num_heads=NUM_HEADS,
        num_layers=NUM_LAYERS,
        ffn_dim=FFN_DIM,
        dropout_rate=DROPOUT_RATE
    )
    
    lr_schedule = tf.keras.optimizers.schedules.CosineDecay(
        initial_learning_rate=LEARNING_RATE,
        decay_steps=EPOCHS * (10_000 // BATCH_SIZE),
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
    
    model.build(input_shape=(None, CONTEXT_WINDOW))
    model.summary()
    
    print(f"\nMODERNIZED MODEL")
    print(f"Total parameters: {model.count_params():,}")
    print(f"Improvements: RMSNorm, SwiGLU, Weight Tying, Scaled Residuals")
    print(f"Mixed precision: {policy.compute_dtype}")
    
    bestLoss = float('inf')
    for epoch in range(EPOCHS):
        print(f"\n=== Epoch {epoch + 1}/{EPOCHS} ===")
        steps_per_epoch = 10_000 // BATCH_SIZE
        
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
            model.save('best_language_model.keras')
            print(f"New best model saved with loss: {bestLoss:.4f}")
    
    wandb.finish()