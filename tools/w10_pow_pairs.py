"""Generate the W-10 pow corpus: pairs of float64 bit patterns, one per line.

The pairs mix random values, integer powers, exact halfway results, values
near 1, subnormals, and arguments near overflow and underflow. The corpus is
reproducible from the seed.
"""
import argparse
import random
import struct


def bits(f):
    return struct.pack(">d", f).hex()


def pairs(seed, count):
    rng = random.Random(seed)
    out = []
    while len(out) < count:
        kind = rng.randrange(8)
        if kind == 0:
            x = struct.unpack(">d", rng.getrandbits(64).to_bytes(8, "big"))[0]
            y = struct.unpack(">d", rng.getrandbits(64).to_bytes(8, "big"))[0]
        elif kind == 1:
            x, y = rng.uniform(-1e3, 1e3), float(rng.randint(-40, 40))
        elif kind == 2:  # results exactly halfway between two doubles
            x, y = float(rng.randrange(2**26 + 1, 2**27, 2)), 2.0
        elif kind == 3:
            x, y = 1 + rng.uniform(-1e-9, 1e-9), rng.uniform(-1e12, 1e12)
        elif kind == 4:
            x, y = rng.uniform(0, 1) * 2.0**-1074 * rng.randint(1, 2**20), rng.uniform(-2, 2)
        elif kind == 5:
            x, y = rng.uniform(1.5, 3), rng.uniform(640, 1100) * rng.choice([1, -1])
        elif kind == 6:
            x, y = rng.uniform(0, 1e6), rng.choice([0.5, 1.5, -0.5, 1 / 3, 2.5])
        else:
            x, y = rng.uniform(-50, 50), rng.uniform(-50, 50)
        out.append(f"{bits(x)} {bits(y)}")
    return out


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--seed", type=int, default=1010)
    parser.add_argument("--count", type=int, default=30000)
    args = parser.parse_args()
    print("\n".join(pairs(args.seed, args.count)))


if __name__ == "__main__":
    main()
