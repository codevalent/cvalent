from dataclasses import dataclass
from typing import Optional

@dataclass
class Config:
    host: str
    port: int
    timeout: Optional[int] = None

def create_config(host: str, port: int) -> Config:
    return Config(host=host, port=port)
