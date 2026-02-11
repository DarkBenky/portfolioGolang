import torch
import torch.nn.functional as F
import wandb
import html
from dataload import build_training_pair, process_training_sample, get_embedding_weight, EMBED_DIM, PAD_ID, tokenizer
from model import SmallTransformerLM

BATCH_SIZE = 1
CONTEXT_SIZE = 512
LEARNING_RATE = 3e-4
MODEL_CONFIG = {
    "n_layers": 32,
    "n_heads": 16,
    "ffn_mult": 4,
    "dropout": 0.1
}

if __name__ == "__main__":
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    wandb.init(project="protoV2", config={
        "context_size": CONTEXT_SIZE,
        "batch_size": BATCH_SIZE,
        "learning_rate": LEARNING_RATE,
        "embedding_dim": EMBED_DIM,
        **MODEL_CONFIG
    })
    embedding_weight = get_embedding_weight()
    model = SmallTransformerLM(
        vocab_size=embedding_weight.size(0),
        embedding_weight=embedding_weight,
        n_layers=MODEL_CONFIG["n_layers"],
        n_heads=MODEL_CONFIG["n_heads"],
        ffn_mult=MODEL_CONFIG["ffn_mult"],
        dropout=MODEL_CONFIG["dropout"]
    ).to(device)

    model_parameters = model.count_parameters()
    model_parameters_m = model_parameters / 1_000_000
    optimizer = torch.optim.AdamW(model.parameters(), lr=LEARNING_RATE)
    model.train()
    wandb.log({
        "model/parameters": model_parameters,
        "model/parameters_m": model_parameters_m,
        "model/d_model": EMBED_DIM,
        "model/n_layers": MODEL_CONFIG["n_layers"],
        "model/n_heads": MODEL_CONFIG["n_heads"],
        "model/ffn_mult": MODEL_CONFIG["ffn_mult"],
        "model/dropout": MODEL_CONFIG["dropout"]
    }, step=0)
    preview_table = wandb.Table(columns=["step", "original", "generated", "combined"], log_mode="MUTABLE")

    batch_texts = []
    batch_step = 0
    best_loss = float("inf")
    for text in process_training_sample(min_words=16, max_words=CONTEXT_SIZE):
        batch_texts.append(text)
        if len(batch_texts) < BATCH_SIZE:
            continue

        batch_step += 1
        input_ids_list = []
        targets_list = []
        for item_text in batch_texts:
            input_ids, target_tokens = build_training_pair(item_text, CONTEXT_SIZE)
            input_ids_list.append(input_ids)
            targets_list.append(target_tokens)
        input_ids = torch.stack(input_ids_list, dim=0).to(device)
        target_tokens = torch.stack(targets_list, dim=0).to(device)
        batch_texts = []

        if batch_step > 1000:
            exit()
            break

        optimizer.zero_grad(set_to_none=True)
        logits = model(input_ids)
        loss = F.cross_entropy(logits.view(-1, logits.size(-1)), target_tokens.view(-1), ignore_index=PAD_ID)
        loss.backward()
        optimizer.step()
        loss_value = loss.item()
        perplexity = torch.exp(loss).item()
        if loss_value < best_loss:
            best_loss = loss_value
            torch.save({
                "model_state_dict": model.state_dict(),
                "loss": best_loss,
                "step": batch_step
            }, "best_model.pt")
        wandb.log({
            "loss": loss_value,
            "perplexity": perplexity
        }, step=batch_step)

        if batch_step % 2 == 0:
            seq_len = input_ids.size(1)
            gen_len = min(64, seq_len)
            preview_input_ids = input_ids[:1, :gen_len]
            with torch.no_grad():
                gen_logits = model(preview_input_ids)
                gen_ids = torch.argmax(gen_logits, dim=-1)[0]
            original_ids = target_tokens[0, :gen_len]
            original_text = tokenizer.decode(original_ids.tolist(), skip_special_tokens=False)
            generated_text = tokenizer.decode(gen_ids.tolist(), skip_special_tokens=False)
            combined_text = original_text + " GEN_START " + generated_text
            preview_table.add_data(batch_step, original_text, generated_text, combined_text)
            original_html = html.escape(original_text)
            generated_html = html.escape(generated_text)
            combined_html = original_html + " <span style=\"color:#d62728;font-weight:bold;\">GEN_START</span> " + generated_html
            wandb.log({
                "preview": preview_table,
                "preview_html": wandb.Html(combined_html)
            }, step=batch_step)

        print(f"Batch {batch_step}: Logits shape: {logits.shape}, Target tokens shape: {target_tokens.shape}")
