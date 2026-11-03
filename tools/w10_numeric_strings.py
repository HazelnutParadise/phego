"""Generate the W-10 numeric string corpus: hex-encoded strings, one per line.

The strings are drawn deterministically from the bytes that matter to PHP
numeric string recognition, so the corpus is reproducible from the seed.
"""
import argparse
import random

ALPHABET = list(" \t\n\r\v\f0123456789+-.eE_xXaIN\x00") + ["0", "1", "9", "9"]
SEEDS = ["9223372036854775807", "9223372036854775808", "-9223372036854775808",
         "2147483647", "2147483648", "-2147483648", "1e308", "1e309", "4.9e-324"]


def corpus(seed, count):
    rng = random.Random(seed)
    out = []
    for _ in range(count):
        if rng.random() < 0.2:
            s = rng.choice(SEEDS)
            s = "".join(rng.choice(ALPHABET) for _ in range(rng.randint(0, 2))) + s + \
                "".join(rng.choice(ALPHABET) for _ in range(rng.randint(0, 3)))
        else:
            s = "".join(rng.choice(ALPHABET) for _ in range(rng.randint(0, 14)))
        out.append(s.encode("latin-1").hex())
    return out


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--seed", type=int, default=1010)
    parser.add_argument("--count", type=int, default=20000)
    args = parser.parse_args()
    print("\n".join(corpus(args.seed, args.count)))


if __name__ == "__main__":
    main()
