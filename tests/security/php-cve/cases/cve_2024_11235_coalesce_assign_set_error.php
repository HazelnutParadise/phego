<?php
// CVE: CVE-2024-11235
// GHSA-rwp7-7vc6-8477: ??= on a property of a temporary freed the temporary
// twice when __set or a typed property raised an error, a use-after-free.
// The upstream tests catch the error; this case lets it end the script.
class Foo {
    public function foo() {
        return $this;
    }
    public function __set($name, $value) {
        echo "__set ", $name, "\n";
        $x = null;
        $x->explode();
    }
}
$foo = new Foo();
$foo->foo()->baz ??= 1;
echo "unreachable\n";
