<?php
trait Registry
{
    public static $items = [];
    private static $created = 0;

    public static function add($item)
    {
        static::$items[] = $item;
        self::$created++;
        return self::$created;
    }

    public static function created()
    {
        return self::$created;
    }

    abstract public function label();

    public function show()
    {
        return "[" . $this->label() . "]";
    }
}

class Users
{
    use Registry;
    public function label()
    {
        return "users";
    }
}

class Posts
{
    use Registry;
    public function label()
    {
        return "posts";
    }
}

echo Users::add("ann"), Users::add("bob"), Posts::add("hello"), "\n";
echo "users created ", Users::created(), " posts created ", Posts::created(), "\n";
foreach (Users::$items as $i => $v) {
    echo "users[", $i, "]=", $v, "\n";
}
foreach (Posts::$items as $i => $v) {
    echo "posts[", $i, "]=", $v, "\n";
}
echo (new Users)->show(), (new Posts)->show(), "\n";

class SubUsers extends Users
{
}
echo "subclass shares: ", SubUsers::add("carl"), " ", Users::created(), "\n";
foreach (SubUsers::$items as $v) {
    echo "sub item ", $v, "\n";
}

trait WithConst
{
    public const VERSION = "1.0";
    public function version()
    {
        return self::VERSION;
    }
}
class HasVersion
{
    use WithConst;
}
echo HasVersion::VERSION, " ", (new HasVersion)->version(), "\n";

trait StaticVarInMethod
{
    public function tick()
    {
        static $n = 0;
        return ++$n;
    }
}
class TickA
{
    use StaticVarInMethod;
}
class TickB
{
    use StaticVarInMethod;
}
$a = new TickA;
$b = new TickB;
echo $a->tick(), $a->tick(), $b->tick(), (new TickA)->tick(), "\n";
