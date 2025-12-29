import random
import string
import math
import re
import signal
from functools import reduce
from operator import mul

# =============================
# CONFIG: how many samples to PRINT per difficulty
# =============================
config = {
    "PRINT_EASY": 10_000,
    "PRINT_MEDIUM": 100,
    "PRINT_HARD": 100,
    "PRINT_INSANE": 100,
    "TIMEOUT_SECONDS": 2,  # Discard samples taking longer than this
    "MAX_STEPS": 50,  # Discard samples with reasoning longer than this
    "CHECK_TOKENIZATION": False
}

# =============================
# TIMEOUT HANDLER
# =============================
class TimeoutError(Exception):
    pass

def timeout_handler(signum, frame):
    raise TimeoutError("Evaluation took too long")

# =============================
# DIFFICULTY PRESETS
# =============================
DIFFICULTIES = {
    "easy": dict(
        min_vars=2, max_vars=5, max_depth=2,
        unary=["abs"],
        binary=[],
        arith=["+", "-", "*", "/"],
        bitwise=False, logic=False, conditional=False
    ),
    "medium": dict(
        min_vars=4, max_vars=9, max_depth=3,
        unary=["sqrt", "abs", "log", "sin", "cos"],
        binary=["pow", "min", "max"],
        arith=["+", "-", "*", "/", "**", "%"],
        bitwise=True, logic=False, conditional=False
    ),
    "hard": dict(
        min_vars=5, max_vars=12, max_depth=5,
        unary=["sqrt", "abs", "log", "sin", "cos", "tan", "exp"],
        binary=["pow", "gcd", "lcm", "min", "max"],
        arith=["+", "-", "*", "/", "**", "%"],
        bitwise=True, logic=True, conditional=True
    ),
    "insane": dict(
        min_vars=7, max_vars=15, max_depth=7,
        unary=["sqrt", "abs", "log", "sin", "cos", "tan", "exp", "floor", "ceil"],
        binary=["pow", "gcd", "lcm", "min", "max"],
        arith=["+", "-", "*", "/", "**", "%"],
        bitwise=True, logic=True, conditional=True
    ),
}

BITWISE_OPS = ["&", "|", "^", "<<", ">>"]
COMPARE_OPS = ["<", ">", "<=", ">=", "==", "!="]
LOGICAL_OPS = ["and", "or"]
CONSTANTS = {"pi": math.pi, "e": math.e}

# =============================
# HELPERS
# =============================
def prod(xs):
    return reduce(mul, xs, 1)

# =============================
# VARIABLE GENERATION
# =============================
def generate_variables(cfg):
    n = random.randint(cfg["min_vars"], cfg["max_vars"])
    names = random.sample(string.ascii_uppercase, n)

    vars = {}
    for name in names:
        if random.random() < 0.5:
            vars[name] = round(random.uniform(1, 10), 3)
        else:
            vars[name] = random.randint(1, 10)
    return vars

# =============================
# EXPRESSION GENERATION
# =============================
def atom(var_names):
    return random.choice(var_names + list(CONSTANTS.keys()))

def gen_expr(var_names, vars, cfg, depth=0):
    if depth >= cfg["max_depth"]:
        return atom(var_names)

    r = random.random()

    if r < 0.25:
        return f"({gen_expr(var_names, vars, cfg, depth+1)} {random.choice(cfg['arith'])} {gen_expr(var_names, vars, cfg, depth+1)})"

    if r < 0.4 and cfg["unary"]:
        return f"{random.choice(cfg['unary'])}({gen_expr(var_names, vars, cfg, depth+1)})"

    if r < 0.55 and cfg["binary"]:
        return f"{random.choice(cfg['binary'])}({gen_expr(var_names, vars, cfg, depth+1)}, {gen_expr(var_names, vars, cfg, depth+1)})"

    if cfg["bitwise"] and r < 0.7:
        ints = [k for k, v in vars.items() if isinstance(v, int)]
        if ints:
            lhs = random.choice(ints)
            if random.choice(BITWISE_OPS) in ["<<", ">>"]:
                return f"({lhs} {random.choice(['<<','>>'])} {random.randint(1,3)})"
            return f"({lhs} {random.choice(['&','|','^'])} {random.choice(ints)})"

    if cfg["logic"] and r < 0.85:
        return f"({gen_expr(var_names, vars, cfg, depth+1)} {random.choice(COMPARE_OPS)} {gen_expr(var_names, vars, cfg, depth+1)})"

    if cfg["conditional"]:
        return (
            f"({gen_expr(var_names, vars, cfg, depth+1)} "
            f"if {gen_expr(var_names, vars, cfg, depth+1)} "
            f"else {gen_expr(var_names, vars, cfg, depth+1)})"
        )

    return atom(var_names)

# =============================
# STEP-BY-STEP REASONING
# =============================
INNER_EXPR = re.compile(r"\([^()]+\)")

def substitute_vars(expr, vars):
    out = expr
    for k, v in vars.items():
        out = re.sub(rf"\b{k}\b", str(v), out)
    return out

def stepwise_evaluate(expr, env):
    steps = []
    current = expr

    while True:
        match = INNER_EXPR.search(current)
        if not match:
            break

        subexpr = match.group(0)
        try:
            value = eval(subexpr, {"__builtins__": {}}, env)
        except Exception:
            break

        steps.append(f"{subexpr} = {value}")
        current = current[:match.start()] + str(value) + current[match.end():]

    final = eval(current, {"__builtins__": {}}, env)
    steps.append(f"{current} = {final}")

    return steps, final

# =============================
# SAMPLE GENERATION
# =============================
def check_tokenization(text, tokenizer):
    """Check if text can be properly tokenized."""
    try:
        encoding = tokenizer.encode(text)
        return True
    except Exception as e:
        print(f"  Tokenization failed: {str(e)[:50]}")
        return False

def generate_sample(difficulty, max_retries=10, tokenizer=None):
    cfg = DIFFICULTIES[difficulty]
    
    for attempt in range(max_retries):
        vars = generate_variables(cfg)
        expr = gen_expr(list(vars.keys()), vars, cfg)

        env = {
            **vars,
            **CONSTANTS,
            "sqrt": math.sqrt,
            "abs": abs,
            "log": math.log,
            "sin": math.sin,
            "cos": math.cos,
            "tan": math.tan,
            "exp": math.exp,
            "floor": math.floor,
            "ceil": math.ceil,
            "pow": pow,
            "gcd": math.gcd,
            "lcm": math.lcm,
            "min": min,
            "max": max,
            "round": round,
            "sum": sum,
            "prod": prod,
        }

        try:
            # Set up timeout alarm
            signal.signal(signal.SIGALRM, timeout_handler)
            signal.alarm(config["TIMEOUT_SECONDS"])
            
            substituted = substitute_vars(expr, vars)
            steps, result = stepwise_evaluate(substituted, env)

            # Cancel the alarm
            signal.alarm(0)

            if isinstance(result, float) and not math.isfinite(result):
                raise ValueError("Result is not finite")
            
            # Check if reasoning has too many steps
            if len(steps) > config["MAX_STEPS"]:
                print(f"  Too many steps ({len(steps)}), retrying...")
                continue
                
        except TimeoutError:
            signal.alarm(0)  # Cancel alarm
            print(f"  Timeout on attempt {attempt + 1}, retrying...")
            continue
        except Exception as e:
            signal.alarm(0)  # Cancel alarm
            continue

        prompt = "Provide correct result of this math code\n\n"
        for k, v in vars.items():
            prompt += f"{k} = {v}\n"
        prompt += f"\n{expr} => ?"

        reasoning = "Reasoning:\n"
        reasoning += "Step 1: Substitute variables\n"
        reasoning += substituted + "\n\n"

        for i, s in enumerate(steps, 2):
            reasoning += f"Step {i}: {s}\n"

        response = reasoning + f"\nFinal Answer:\nThe solution is: {result}"

        # Check tokenization if enabled
        if config["CHECK_TOKENIZATION"] and tokenizer is not None:
            full_text = prompt + "\n" + response
            if not check_tokenization(full_text, tokenizer):
                print(f"  Tokenization check failed, retrying...")
                continue

        return {
            "difficulty": difficulty,
            "prompt": prompt,
            "response": response
        }
    
    raise RuntimeError(f"Failed to generate valid sample after {max_retries} attempts")

if __name__ == "__main__":
    random.seed(42)

    # Load tokenizer if tokenization check is enabled
    tokenizer = None
    if config["CHECK_TOKENIZATION"]:
        try:
            from tokenizers import Tokenizer
            tokenizer = Tokenizer.from_file(config["TOKENIZER_PATH"])
            print(f"Loaded tokenizer from {config['TOKENIZER_PATH']}")
        except Exception as e:
            print(f"Warning: Could not load tokenizer: {e}")
            print("Continuing without tokenization checks...")
            config["CHECK_TOKENIZATION"] = False

    data = []

    for diff, count in [
        ("easy", config["PRINT_EASY"]),
        ("medium", config["PRINT_MEDIUM"]),
        ("hard", config["PRINT_HARD"]),
        ("insane", config["PRINT_INSANE"]),]:

        successful = 0
        attempts = 0
        max_total_attempts = count * 20  # Allow up to 20x attempts
        
        while successful < count and attempts < max_total_attempts:
            attempts += 1
            try:
                sample = generate_sample(diff, tokenizer=tokenizer)
                data.append(sample)
                successful += 1
                print(f"Generated {successful}/{count} for difficulty '{diff}'")
            except RuntimeError as e:
                print(f"  Skipping sample after failed attempts, continuing... ({successful}/{count} successful)")
                continue
        
        if successful < count:
            print(f"Warning: Only generated {successful}/{count} samples for '{diff}' after {attempts} attempts")

    import json
    with open("math_samples.json", "w") as f:
        json.dump(data, f, indent=2)
    
    print(f"\nSuccessfully generated {len(data)} samples!")