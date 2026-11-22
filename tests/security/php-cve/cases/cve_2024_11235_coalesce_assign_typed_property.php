<?php
// CVE: CVE-2024-11235
// GHSA-rwp7-7vc6-8477, the typed-property variant of
// Zend/tests/ghsa-rwp7-7vc6-8477_003.phpt without the catch.
class Foo {
    public int $prop;
}
function newFoo() {
    return new Foo();
}
newFoo()->prop ??= 'foo';
echo "unreachable\n";
