<?php
class Tracker
{
    public static $log = "";

    public function __construct(public $id)
    {
    }

    public function __destruct()
    {
        Tracker::$log .= "[" . $this->id . "]";
        echo "~", $this->id, " ";
    }
}

echo "a ";
$t = new Tracker(1);
echo "b ";
$t = new Tracker(2);
echo "c ";
echo "d", "\n";
$x = new Tracker(3);
$y = $x;
$x = new Tracker(4);
echo "e ";
$y = null;
echo "f\n";
echo "log so far: ", Tracker::$log, "\n";

class Parentish
{
    public function __destruct()
    {
        echo "parent destructor of ", static::class, "\n";
    }
}
class Childish extends Parentish
{
    public function __destruct()
    {
        echo "child destructor first\n";
        parent::__destruct();
    }
}
class NoOwn extends Parentish
{
}
$c = new Childish;
unset($c);
$n = new NoOwn;
unset($n);
$z = new Tracker(5);
$z->__destruct();
echo "\nexplicit destructor call does not destroy\n";
unset($z);
echo "\nend\n";
