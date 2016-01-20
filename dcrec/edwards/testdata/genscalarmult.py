import os
from ed25519 import *

f = open("scalarmulttests.dat",'w')

numTests = 50
for i in range(0,numTests):
    rand_string = os.urandom(32)
    try: 
        p = decodepoint(rand_string)
    except: 
        continue
    rand_string = os.urandom(32)
    s = decodeint(rand_string)
    
    mult = scalarmult(p, s)
    
    f.write("ScalarMultVectorHex{")
    # Point to multiply
    f.write('\"')
    f.write("".join("{:02x}".format(ord(c)) for c in encodepoint(p)))
    f.write('\"')
    f.write(',')
    # Scalar to multiply by
    f.write('\"')
    f.write("".join("{:02x}".format(ord(c)) for c in encodeint(s)))
    f.write('\"')
    f.write(',')
    # Resulting point
    f.write('\"')
    f.write("".join("{:02x}".format(ord(c)) for c in encodepoint(mult)))
    f.write('\"')
    f.write('},\n')

f.close()
