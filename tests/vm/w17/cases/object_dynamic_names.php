<?php
class Config
{
    public $host = "localhost";
    public $port = 80;
    public $list = [];

    public function get($k)
    {
        return $this->$k;
    }

    public function set($k, $v)
    {
        $this->$k = $v;
        return $this;
    }

    public function greet()
    {
        return "hi";
    }

    public static function make()
    {
        return "made";
    }
}

$c = new Config;
$prop = "host";
echo $c->$prop, "\n";
echo $c->{"po" . "rt"}, "\n";
$c->$prop = "example.org";
echo $c->get("host"), "\n";
$c->set("port", 8080)->set("timeout", 30);
echo $c->port, " ", $c->timeout, "\n";
$names = ["host", "port"];
foreach ($names as $n) {
    echo $n, "=", $c->$n, " ";
}
echo "\n";
echo "element name: ", $c->{$names[1]}, "\n";
$l = "list";
$c->$l[] = "first";
$c->{$l}["k"] = "keyed";
echo $c->list[0], " ", $c->list["k"], "\n";
$m = "greet";
echo $c->$m(), "\n";
echo $c->{"gr" . "eet"}(), "\n";
$s = "make";
echo Config::$s(), "\n";
$cls = "Config";
echo $cls::make(), " ", $cls::$s(), "\n";
$num = 12;
$c->$num = "numeric name";
echo $c->{"12"}, "\n";
$c->{"with space"} = "spaced";
echo $c->{"with space"}, "\n";
$c->{""} = "x";
