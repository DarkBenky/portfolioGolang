from dotenv import load_dotenv
import os

load_dotenv()  # Load .env file

BACKEND_GO_PORT = os.getenv("BACKEND_GO_PORT") 
BACKEND_GO = f"http://localhost:{BACKEND_GO_PORT}"
BACKEND_PYTHON_PORT = os.getenv("BACKEND_PYTHON_PORT")
BACKEND_PYTHON = f"http://localhost:{BACKEND_PYTHON_PORT}"