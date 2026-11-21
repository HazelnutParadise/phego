<?php
echo "start\n";
interface Speaks
{
    public function speak();
    public function volume();
}
class Mute implements Speaks
{
    public function speak()
    {
    }
}
