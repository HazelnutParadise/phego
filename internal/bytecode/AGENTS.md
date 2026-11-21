# Rules for `internal/bytecode`

Before changing a rule here, read its history with `git log -S '<words from the line>'`.

- Validating exception handlers: two handlers are either fully disjoint, or both the protected range and the body range of the inner one lie inside the same single region of the outer one. Check the two ranges together; checking them separately allows cross-scope stitching, and treating all handler bodies as disjoint rejects `try`/`catch` inside `finally`. `EndFinally` belongs to the innermost handler body of kind Finally. A compiler continuation after `Leave` reloads locals or redefines temporaries, because definitions do not survive `finally`.
