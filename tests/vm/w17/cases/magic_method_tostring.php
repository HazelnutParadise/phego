<?php
class Money
{
    public function __construct(private $amount, private $cur = "EUR")
    {
    }

    public function __toString(): string
    {
        return $this->amount . " " . $this->cur;
    }
}

class Name
{
    public function __construct(public $n)
    {
    }
    public function __toString()
    {
        return $this->n;
    }
}

$m = new Money(5);
echo $m, "\n";
print $m;
print "\n";
echo "concat: " . $m . "\n";
echo "interpolation: $m and {$m}\n";
$s = (string)$m;
echo "cast: ", $s, "\n";
$x = "start ";
$x .= $m;
echo "concat assign: ", $x, "\n";
echo "== string: ", ($m == "5 EUR" ? "equal" : "not"), "\n";
echo "== other string: ", ($m == "6 EUR" ? "equal" : "not"), "\n";
echo "=== string: ", ($m === "5 EUR" ? "identical" : "not identical"), "\n";
echo "<=> string: ", (new Name("b") <=> "a"), " ", (new Name("a") <=> "b"), "\n";
echo "string first: ", ("5 EUR" == $m ? "equal" : "not"), "\n";

function needs_string(string $s)
{
    return "got " . $s;
}
echo needs_string(new Name("param")), "\n";
function ret_string($o): string
{
    return $o;
}
echo ret_string(new Name("return")), "\n";
$arr = ["5 EUR" => "found"];
echo "as key via cast: ", $arr[(string)$m], "\n";
class class_child extends Name
{
}
echo "inherit: ", new class_child("child"), "\n";
echo "explicit call: ", $m->__toString(), "\n";
echo "empty string: [", new Name(""), "]\n";
echo "numeric string comparison: ", (new Name("10") == "1e1" ? "equal" : "not"), "\n";
