"""Conservative, bounded parser for PHP 8.5.12 PHPT ARGS shell words."""

MAX_ARGS = 64 * 1024
MAX_WORDS = 4096


def parse_args(data):
    if not isinstance(data, bytes):
        raise ValueError("ARGS must be bytes")
    if len(data) > MAX_ARGS:
        raise ValueError("ARGS exceeds 64 KiB")
    nul = data.find(b"\0")
    if nul >= 0:
        raise ValueError(f"ARGS contains NUL at byte {nul + 1}")
    # Trailing LF ends the shell command; CR remains argument data.
    data = data.rstrip(b"\n")
    args, word = [], bytearray()
    started = False
    quote = None
    quote_start = 0
    i = 0
    while i < len(data):
        c = data[i]
        if quote == 39:
            if c == 39:
                quote = None
            else:
                word.append(c)
            started = True
            i += 1
            continue
        if quote == 34:
            if c == 34:
                quote = None
            elif c == 92:
                if i + 1 >= len(data):
                    raise ValueError(f"ARGS ends with an incomplete escape at byte {i + 1}")
                nxt = data[i + 1]
                if nxt == 10:
                    raise ValueError(f"ARGS contains an escaped newline at byte {i + 1}")
                if nxt in b'"\\$`':
                    word.append(nxt)
                else:
                    word.extend((c, nxt))
                i += 2
                started = True
                continue
            elif c in (36, 96):
                raise ValueError(f"ARGS contains unsupported shell expansion at byte {i + 1}")
            else:
                word.append(c)
            started = True
            i += 1
            continue
        if c in (32, 9):
            if started:
                args.append(bytes(word)); word.clear(); started = False
            i += 1
            continue
        if c in (10, 13, 11, 12):
            if c == 10 and all(x == 10 for x in data[i:]):
                break
            if c == 10:
                raise ValueError(f"ARGS contains an internal newline at byte {i + 1}")
            word.append(c); started = True; i += 1; continue
        if c in (39, 34):
            quote = c; quote_start = i; started = True; i += 1; continue
        if c == 92:
            if i + 1 >= len(data):
                raise ValueError(f"ARGS ends with an incomplete escape at byte {i + 1}")
            if data[i + 1] == 10:
                raise ValueError(f"ARGS contains an escaped newline at byte {i + 1}")
            word.append(data[i + 1]); started = True; i += 2; continue
        if c in b"$`*?[]{}|&;<>()#~":
            raise ValueError(f"ARGS contains unsupported shell syntax at byte {i + 1}")
        word.append(c); started = True; i += 1
    if quote is not None:
        raise ValueError(f"ARGS contains an unclosed quote at byte {quote_start + 1}")
    if started:
        args.append(bytes(word))
    if len(args) > MAX_WORDS:
        raise ValueError("ARGS exceeds 4096 arguments")
    return tuple(args)
