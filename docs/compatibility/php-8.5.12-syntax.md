# PHP 8.5.12 syntax coverage

This list describes what `internal/frontend` accepts for the PHP 8.5.12 profile. Acceptance, rejection, and the reported error line were compared with the PHP 8.5.12 reference for all 5449 `--FILE--` sections of php-src's Zend and tokenizer tests; all 5449 agree. Other releases need their own profiles and are not covered.

## Supported

- Templates: inline HTML, `<?php`, `<?=`, short `<?` tags when `short_open_tag` is on, `?>` as a statement end, `__halt_compiler()`.
- Tokens: every PHP 8.5.12 token, including the two ampersand tokens, the pipe operator `|>`, `(void)`, asymmetric visibility such as `private(set)`, attributes, flexible heredoc and nowdoc, and integer literals sized by the 32- or 64-bit platform profile.
- Declarations: namespaces (statement and braced, reserved-word names), `use` with functions, constants, groups, and aliases, top-level constants with attributes, functions, classes, abstract/final/readonly classes, interfaces, traits with adaptation blocks, enums with backing types and cases, anonymous classes with attributes and modifiers.
- Members: typed properties, constructor promotion, property hooks, typed class constants, methods, abstract members, union, intersection, DNF, nullable, and `static` return types.
- Statements: all control structures in both brace and alternative syntax, `declare`, `try`/`catch`/`finally` with multi-catch and optional variables, `goto` and labels, `global`, `static`, `echo`, `unset`, `(void)` statements and `for` expressions.
- Expressions: PHP 8.5.12 operator precedence and associativity, non-associative comparisons, assignment to variables in any position, by-reference assignment, list and short-array destructuring with keys and nesting, closures, arrow functions, `static` closures, `match`, `throw` expressions, `new` with arbitrary expressions and argument-less member access rules, `clone` with arguments, first-class callable syntax, named arguments, spread, nullsafe access, dynamic class constant fetch, string interpolation in all forms, `yield`, `yield from`, `include`/`require`, `print`, `exit`/`die`, `isset`, `empty`, `eval`.
- Errors PHP raises while building the tree: modifier conflicts and misplaced modifiers, `declare(encoding=...)` non-literal values, `__halt_compiler()` inside braces, heredoc indentation errors, the removed `(real)` cast, unterminated comments.

## Known gaps

- The nesting limit protects the Go stack but is not PHP's: PHP's parser stack accepts up to 9995 nested parentheses yet only 2499 nested if statements, while Phego allows about 5000 parentheses and up to 10000 levels of most other constructs. Such input is rejected with a different message (PHP reports "memory exhausted").

- Of 16320 token-level mutants of the corpus, all receive PHP's accept or reject decision and 16318 the same error line; two heredoc indentation errors are reported on a different line.

- Error messages are located like PHP's but do not reproduce Bison's "expecting ..." lists.
- Tree shapes are checked by unit tests; their meaning is verified only once execution exists.
- The reference ran with `short_open_tag` on; the off setting is covered by unit tests only.
- Compile-time errors outside the parser, such as duplicate declarations or invalid `break` levels, belong to compilation.
