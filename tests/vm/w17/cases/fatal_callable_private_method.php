<?php
// A private method cannot be turned into a callable from outside the class.
// The error comes at creation time.

class A
{
    private function priv()
    {
        return "private result";
    }

    public function viaInside()
    {
        $c = $this->priv(...);
        return $c();
    }
}

$a = new A();
echo "public path: ", $a->viaInside(), "\n";

echo "before the forbidden creation\n";

$bad = $a->priv(...);

echo "after creation, not reached\n";
