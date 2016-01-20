import sys
from ed25519 import *

s1 = sys.argv[1].decode("hex")
P1 = decodepoint(s1)

s2 = sys.argv[2].decode("hex")
P2 = decodepoint(s2)

P = edwards(P1, P2)
encodepointhex(P)
