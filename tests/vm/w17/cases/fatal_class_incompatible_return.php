<?php
echo "start\n";
interface Maker
{
    public function make(): int;
}
class Bad implements Maker
{
    public function make(): string
    {
        return "";
    }
}
