<?php
class C {
    public function join($a, $b = "B", ...$rest) {
        $out = $a . "|" . $b;
        foreach ($rest as $r) {
            $out = $out . "|" . $r;
        }
        return $out;
    }

    public function swap(&$x, &$y) {
        $t = $x;
        $x = $y;
        $y = $t;
        return "swapped";
    }
}

$c = new C;
$j = $c->join(...);
echo "named_out_of_order: ", $j(b: "bb", a: "aa"), "\n";
echo "default_used: ", $j("only"), "\n";
echo "extra_positional: ", $j("p", "q", "r", "s"), "\n";
echo "named_to_variadic: ", $j("a1", b: "b1"), "\n";

$arr = ["x", "y", "z"];
echo "unpack_positional: ", $j(...$arr), "\n";

$named = ["a" => "NA", "b" => "NB"];
echo "unpack_named: ", $j(...$named), "\n";
echo "unknown_named_to_variadic: ", $j(...["a" => "A1", "zz" => "Z"]), "\n";

$sw = $c->swap(...);
$p = 1;
$q = 2;
echo "byref_result: ", $sw($p, $q), " p=", $p, " q=", $q, "\n";
$sw($p, $q);
echo "byref_second_call: p=", $p, " q=", $q, "\n";
