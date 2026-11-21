<?php
class Item
{
}
class Service
{
    public function handle(Item $item, $name)
    {
        return $this->inner($item, [$item], $name);
    }

    protected function inner($o, $list, $s)
    {
        return $o->missing();
    }
}
echo "start\n";
(new Service)->handle(new Item, "a fairly long string argument that gets truncated");
