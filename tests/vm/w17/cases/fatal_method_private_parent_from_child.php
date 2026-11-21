<?php
class Base
{
    private function secret()
    {
        return "s";
    }
}
class Child extends Base
{
    public function peek()
    {
        return $this->secret();
    }
}
echo "peeking\n";
(new Child)->peek();
