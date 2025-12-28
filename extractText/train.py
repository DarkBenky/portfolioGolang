import tensorflow as tf
import wandb
from wandb.integration.keras import WandbMetricsLogger
from dataGen import DataGenerator

class TransformerBlock(tf.keras.layers.Layer):
    def __init__(self, d_model, num_heads, dropout=0.1):
        super().__init__()
        self.attn = tf.keras.layers.MultiHeadAttention(
            num_heads=num_heads,
            key_dim=d_model // num_heads,
            dropout=dropout
        )
        self.ff = tf.keras.Sequential([
            tf.keras.layers.Dense(4 * d_model, activation="gelu"),
            tf.keras.layers.Dropout(dropout),
            tf.keras.layers.Dense(d_model),
            tf.keras.layers.Dropout(dropout),
        ])
        self.norm1 = tf.keras.layers.LayerNormalization(epsilon=1e-6)
        self.norm2 = tf.keras.layers.LayerNormalization(epsilon=1e-6)
        self.dropout1 = tf.keras.layers.Dropout(dropout)
        self.dropout2 = tf.keras.layers.Dropout(dropout)

    def call(self, x, training=False):
        attn_out = self.attn(x, x, training=training)
        attn_out = self.dropout1(attn_out, training=training)
        x = self.norm1(x + attn_out)
        
        ff_out = self.ff(x, training=training)
        ff_out = self.dropout2(ff_out, training=training)
        return self.norm2(x + ff_out)

class Transformer(tf.keras.Model):
    def __init__(self, vocab_size, d_model, num_heads, num_layers, max_len, dropout=0.1):
        super().__init__()
        self.d_model = d_model
        self.embed = tf.keras.layers.Embedding(vocab_size, d_model)
        self.pos_encoding = self.positional_encoding(max_len, d_model)
        self.dropout = tf.keras.layers.Dropout(dropout)
        self.blocks = [TransformerBlock(d_model, num_heads, dropout) for _ in range(num_layers)]
        self.norm = tf.keras.layers.LayerNormalization(epsilon=1e-6)
        self.out = tf.keras.layers.Dense(vocab_size)

    def positional_encoding(self, max_len, d_model):
        import numpy as np
        pos = np.arange(max_len)[:, np.newaxis]
        i = np.arange(d_model)[np.newaxis, :]
        angle_rates = 1 / np.power(10000, (2 * (i // 2)) / np.float32(d_model))
        angle_rads = pos * angle_rates
        angle_rads[:, 0::2] = np.sin(angle_rads[:, 0::2])
        angle_rads[:, 1::2] = np.cos(angle_rads[:, 1::2])
        return tf.cast(angle_rads[np.newaxis, ...], dtype=tf.float32)

    def call(self, tokens, training=False):
        seq_len = tf.shape(tokens)[1]
        x = self.embed(tokens)
        x *= tf.cast(tf.math.sqrt(tf.cast(self.d_model, tf.float32)), x.dtype)
        x += tf.cast(self.pos_encoding[:, :seq_len, :], x.dtype)
        x = self.dropout(x, training=training)
        
        for block in self.blocks:
            x = block(x, training=training)
        
        x = self.norm(x)
        return self.out(x[:, -1, :])

if __name__ == '__main__':
    CONTEXT_WINDOW = 512
    D_MODEL = 384
    NUM_HEADS = 12
    NUM_LAYERS = 32
    BATCH_SIZE = 1
    LEARNING_RATE = 3e-4
    EPOCHS = 100
    
    tf.keras.mixed_precision.set_global_policy('mixed_float16')
    
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
        }
    )
    
    data_gen = DataGenerator('extractedTexts.json', 'tokenizer.json')
    vocab_size = data_gen.vocab_size
    wandb.config.update({"vocab_size": vocab_size})
    
    model = Transformer(
        vocab_size=vocab_size,
        d_model=D_MODEL,
        num_heads=NUM_HEADS,
        num_layers=NUM_LAYERS,
        max_len=CONTEXT_WINDOW,
        dropout=0.1
    )
    
    optimizer = tf.keras.optimizers.Adam(learning_rate=LEARNING_RATE)
    loss_fn = tf.keras.losses.SparseCategoricalCrossentropy(from_logits=True)
    model.compile(optimizer=optimizer, loss=loss_fn, metrics=['accuracy'])
    
    dummy_input = tf.zeros((1, CONTEXT_WINDOW), dtype=tf.int32)
    _ = model(dummy_input, training=False)
    
    model.summary()
    trainable_params = sum([tf.size(w).numpy() for w in model.trainable_weights])
    print(f"Total parameters: {trainable_params:,}")
    
    train_dataset = tf.data.Dataset.from_generator(
        lambda: data_gen.generate_batches(BATCH_SIZE, CONTEXT_WINDOW),
        output_signature=(
            tf.TensorSpec(shape=(BATCH_SIZE, CONTEXT_WINDOW), dtype=tf.int32),
            tf.TensorSpec(shape=(BATCH_SIZE,), dtype=tf.int32)
        )
    ).prefetch(tf.data.AUTOTUNE)
    
    wandb_metrics_logger = WandbMetricsLogger()
    
    print("Starting training...")
    model.fit(
        train_dataset,
        epochs=EPOCHS,
        steps_per_epoch=1000,
        callbacks=[wandb_metrics_logger]
    )
    
    wandb.finish()
    print("Training complete!")
