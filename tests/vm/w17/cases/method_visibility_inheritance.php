<?php
class Base
{
    public function run()
    {
        return $this->priv() . " " . $this->prot() . " " . $this->pub();
    }

    private function priv()
    {
        return "Base::priv";
    }

    protected function prot()
    {
        return "Base::prot";
    }

    public function pub()
    {
        return "Base::pub";
    }

    public function callOn(Base $other)
    {
        return $other->priv() . " " . $other->prot();
    }
}

class Child extends Base
{
    public function priv()
    {
        return "Child::priv";
    }

    protected function prot()
    {
        return "Child::prot";
    }

    public function pub()
    {
        return "Child::pub via " . parent::prot();
    }

    public function sibling(Other $o)
    {
        return $o->prot();
    }
}

class Other extends Base
{
    protected function prot()
    {
        return "Other::prot";
    }
}

$c = new Child;
echo $c->run(), "\n";
echo $c->priv(), "\n";
echo (new Base)->callOn($c), "\n";
echo $c->sibling(new Other), "\n";
echo (new Other)->run(), "\n";

class StaticVis
{
    private static function hidden()
    {
        return "hidden static";
    }

    public static function reveal()
    {
        return self::hidden() . " / " . static::hidden();
    }
}
echo StaticVis::reveal(), "\n";
