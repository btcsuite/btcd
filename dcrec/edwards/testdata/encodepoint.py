import sys
from ed25519 import *

P = []
x = int(sys.argv[1])
P.append(x)
y = int(sys.argv[2])
P.append(y)
encodepointhex(P)
