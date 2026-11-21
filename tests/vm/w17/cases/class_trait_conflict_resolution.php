<?php
trait A
{
    public function hello()
    {
        return "A::hello";
    }
    public function shared()
    {
        return "A::shared";
    }
}

trait B
{
    public function hello()
    {
        return "B::hello";
    }
    public function shared()
    {
        return "B::shared";
    }
    private function secret()
    {
        return "B::secret";
    }
}

class Talker
{
    use A, B {
        A::hello insteadof B;
        B::hello as helloB;
        B::shared insteadof A;
        A::shared as protected sharedA;
        secret as public revealed;
    }

    public function callShared()
    {
        return $this->sharedA();
    }
}

$t = new Talker;
echo $t->hello(), "\n";
echo $t->helloB(), "\n";
echo $t->shared(), "\n";
echo $t->callShared(), "\n";
echo $t->revealed(), "\n";

trait Visible
{
    public function open()
    {
        return "open";
    }
}
class Hides
{
    use Visible {
        open as protected;
    }
    public function viaInside()
    {
        return "inside " . $this->open();
    }
}
echo (new Hides)->viaInside(), "\n";

class AliasKeepsOriginal
{
    use Visible {
        open as private closedAlias;
    }
    public function both()
    {
        return $this->open() . "+" . $this->closedAlias();
    }
}
echo (new AliasKeepsOriginal)->both(), "\n";
echo (new AliasKeepsOriginal)->open(), "\n";
