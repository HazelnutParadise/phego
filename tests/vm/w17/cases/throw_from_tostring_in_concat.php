<?php
// throw_from_tostring_in_concat: __toString that throws, during concatenation, echo, interpolation and loose comparison
class TS {
    public function __toString(): string {
        throw new Exception("tostring failed");
    }
}

$o = new TS();

try {
    $s = "a" . $o . "b";
    echo "not reached concat\n";
} catch (Exception $e) {
    echo "concat: ", $e->getMessage(), "\n";
}

try {
    echo $o;
    echo "not reached echo\n";
} catch (Exception $e) {
    echo "echo: ", $e->getMessage(), "\n";
}

try {
    $t = "x{$o}y";
    echo "not reached interpolation\n";
} catch (Exception $e) {
    echo "interpolation: ", $e->getMessage(), "\n";
}

try {
    $eq = ($o == "a");
    echo "not reached compare\n";
} catch (Exception $e) {
    echo "compare: ", $e->getMessage(), "\n";
}

$same = ($o === "a");
echo "identity_no_tostring: ", ($same ? 1 : 0), "\n";

$ok = "pre" . "post";
echo "normal_concat: ", $ok, "\n";
echo "end\n";
