<?php
echo "start\n";
class Bad
{
    public function __toString(): int
    {
        return 1;
    }
}
