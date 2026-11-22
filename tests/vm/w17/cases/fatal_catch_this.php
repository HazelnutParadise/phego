<?php
// fatal_catch_this: $this cannot be re-assigned as a catch variable (compile error)
class Holder
{
    public function run()
    {
        try {
            throw new Exception("x");
        } catch (Exception $this) {
            echo "caught\n";
        }
    }
}

echo "not reached\n";
