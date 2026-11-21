<?php
class A
{
    public $tag = "A tag";

    public function who()
    {
        return "A::who sees " . $this->tag . " in " . static::class;
    }

    public static function sw()
    {
        return "A::sw " . static::class;
    }
}

class B extends A
{
    public $tag = "B tag";

    public function who()
    {
        return "B::who > " . parent::who();
    }

    public function callAStatically()
    {
        return A::who();
    }

    public static function sw()
    {
        return "B::sw > " . parent::sw();
    }
}

class C extends B
{
    public function who()
    {
        return "C::who > " . parent::who();
    }
}

$c = new C;
echo $c->who(), "\n";
echo "ancestor call keeps \$this: ", $c->callAStatically(), "\n";
echo C::sw(), "\n";
echo B::sw(), "\n";
