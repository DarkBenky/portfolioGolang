from dotenv import load_dotenv
import os

load_dotenv()

DEV_MODE = os.getenv("DEV_MODE", "true") == "true"
BACKEND_GO_PORT = os.getenv("BACKEND_GO_PORT")
BACKEND_PYTHON_PORT = os.getenv("BACKEND_PYTHON_PORT")
SERVER_HOST = os.getenv("SERVER_HOST", "localhost")
OPENROUTER_API_KEY = os.getenv("OPENROUTER_API_KEY", "")
OPENROUTER_MODEL = os.getenv("OPENROUTER_MODEL", "qwen/qwen3.7-flash")
PYTHON_API_KEY = os.getenv("PYTHON_API_KEY", "")

if DEV_MODE:
    BACKEND_GO = f"http://localhost:{BACKEND_GO_PORT}"
    BACKEND_PYTHON = f"http://localhost:{BACKEND_PYTHON_PORT}"
else:
    BACKEND_GO = f"http://{SERVER_HOST}:{BACKEND_GO_PORT}"
    BACKEND_PYTHON = f"http://{SERVER_HOST}:{BACKEND_PYTHON_PORT}"

print(f"Python Backend Config: DEV_MODE={DEV_MODE}, GO={BACKEND_GO}, PYTHON={BACKEND_PYTHON}")