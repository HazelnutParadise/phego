<?php
class QueryBuilder
{
    private $parts = [];

    public static function table($t)
    {
        $q = new static;
        $q->parts[] = "FROM " . $t;
        return $q;
    }

    public function where($c)
    {
        $this->parts[] = "WHERE " . $c;
        return $this;
    }

    public function limit($n)
    {
        $copy = clone $this;
        $copy->parts[] = "LIMIT " . $n;
        return $copy;
    }

    public function build()
    {
        $s = "SELECT *";
        foreach ($this->parts as $p) {
            $s .= " " . $p;
        }
        return $s;
    }
}

$q = QueryBuilder::table("users")->where("id > 1");
echo $q->build(), "\n";
$limited = $q->limit(5);
echo $limited->build(), "\n";
echo "original untouched: ", $q->build(), "\n";
echo QueryBuilder::table("a")->where("x")->where("y")->limit(1)->build(), "\n";
echo (new QueryBuilder)->where("only")->build(), "\n";

class Node
{
    public $children = [];
    public function __construct(public $name)
    {
    }
    public function add($name)
    {
        $c = new Node($name);
        $this->children[] = $c;
        return $c;
    }
    public function path($depth = 0)
    {
        $s = $depth . ":" . $this->name;
        foreach ($this->children as $c) {
            $s .= " " . $c->path($depth + 1);
        }
        return $s;
    }
}
$root = new Node("root");
$root->add("a")->add("a1")->add("a1x");
$root->add("b");
echo $root->path(), "\n";
echo $root->children[0]->children[0]->name, "\n";
