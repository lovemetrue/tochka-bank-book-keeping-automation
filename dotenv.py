"""Минимальная замена python-dotenv (на хосте ВМ пакета нет): нужна, чтобы работал notify.py."""
import os


def load_dotenv(path):
    if not os.path.exists(path):
        return False
    for line in open(path, encoding="utf-8"):
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            os.environ.setdefault(k.strip(), v.strip().strip("\"'"))
    return True
