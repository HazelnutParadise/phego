<?php
// fatal_throw_uncaught_destructor_at_shutdown: an object alive at script end throws from its destructor during shutdown
class Late
{
    public function __destruct()
    {
        echo "late destructor\n";
        throw new Exception("at shutdown");
    }
}

class Quiet
{
    public function __destruct()
    {
        echo "quiet destructor\n";
    }
}

$quiet = new Quiet();
$late = new Late();
echo "end of script\n";
