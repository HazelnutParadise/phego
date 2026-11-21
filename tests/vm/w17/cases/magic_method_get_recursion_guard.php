<?php
class Guard
{
    public function __get($name)
    {
        echo "__get(", $name, ") ";
        return $this->$name;
    }

    public function __set($name, $value)
    {
        echo "__set(", $name, ") ";
        $this->$name = $value . "!";
    }
}

$g = new Guard;
echo "[", $g->foo, "]\n";
$g->bar = "v";
echo "\n", $g->bar, "\n";

class Other
{
    public function __get($name)
    {
        echo "outer __get(", $name, ") ";
        if ($name === "a") {
            return $this->b;
        }
        return "value of " . $name;
    }
}
$o = new Other;
echo $o->a, "\n";
