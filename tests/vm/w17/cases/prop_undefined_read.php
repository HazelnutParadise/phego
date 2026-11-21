<?php
class Sparse
{
    public $known = "known";
    private $hidden = "hidden";

    public function readHiddenOfThis()
    {
        return $this->hidden;
    }

    public function readUnknown()
    {
        return $this->unknown;
    }
}

$s = new Sparse;
echo "known: ", $s->known, "\n";
echo "unknown outside: [", $s->nothing, "]\n";
echo "unknown inside: [", $s->readUnknown(), "]\n";
$v = $s->nothing;
echo "value is null: ", ($v === null ? "yes" : "no"), "\n";
echo "concat with undefined: [", "x" . $s->nothing, "]\n";
echo "arith with undefined: ", $s->nothing + 1, "\n";
echo "element of undefined prop: [", $s->nothing["k"], "]\n";
echo "prop of undefined prop: [", $s->nothing->deeper, "]\n";
$name = "dynamicName";
echo "dynamic name: [", $s->$name, "]\n";
echo "coalesce has no warning: ", ($s->nothing ?? "fallback"), "\n";

class Child extends Sparse
{
    public function readParentPrivate()
    {
        return $this->hidden;
    }
}
echo "parent private from child: [", (new Child)->readParentPrivate(), "]\n";
