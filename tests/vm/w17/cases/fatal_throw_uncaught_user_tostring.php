<?php
// fatal_throw_uncaught_user_tostring: uncaught user exception that overrides __toString
class Named extends Exception
{
    public function __toString(): string
    {
        return "custom text for " . $this->getMessage();
    }
}

function fail()
{
    throw new Named("boom in fail");
}

echo "start\n";
fail();
echo "never\n";
