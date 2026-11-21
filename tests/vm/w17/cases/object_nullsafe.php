<?php
class Node
{
    public $next = null;
    public $value;

    public function __construct($value)
    {
        $this->value = $value;
    }

    public function getNext()
    {
        echo "getNext on ", $this->value, "\n";
        return $this->next;
    }

    public function label($suffix)
    {
        return "node " . $this->value . $suffix;
    }
}

function arg($v)
{
    echo "evaluated arg ", $v, "\n";
    return $v;
}

$a = new Node("a");
$a->next = new Node("b");

echo "prop: ", $a?->value, "\n";
echo "chain: ", $a?->next?->value, "\n";
echo "chain to null: [", $a?->next?->next?->value, "]\n";
echo "method: ", $a?->getNext()?->label("!"), "\n";
echo "method on null result: [", $a->getNext()?->getNext()?->label(arg("skipped")), "]\n";
$n = null;
echo "null start: [", $n?->value, "]\n";
echo "short-circuit args: [", $n?->label(arg("never")), "]\n";
echo "short-circuit whole chain: [", $n?->next->value->deeper, "]\n";
echo "short-circuit method chain: [", $n?->getNext()->getNext()->label("x"), "]\n";
echo "coalesce: ", ($n?->value ?? "default"), "\n";
echo "isset: ", (isset($a?->next?->value) ? "yes" : "no"), " ", (isset($n?->value) ? "yes" : "no"), "\n";
echo "empty: ", (empty($n?->value) ? "yes" : "no"), "\n";
$r = $n?->value;
echo "result is null: ", ($r === null ? "yes" : "no"), "\n";
echo "on undefined variable: [", $missing?->value, "]\n";
$arr = ["obj" => $a, "none" => null];
echo "array element: ", $arr["obj"]?->value, " [", $arr["none"]?->value, "]\n";
echo "nullsafe then array: [", $n?->list[0], "]\n";
