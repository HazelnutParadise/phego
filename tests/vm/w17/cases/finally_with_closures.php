<?php
// finally_with_closures: finally blocks in closures and methods; a closure called from a finally block throws and is caught there
class Runner
{
    public function run($fail)
    {
        try {
            echo "method try\n";
            if ($fail) {
                throw new RuntimeException("method failure");
            }
            return "method ok";
        } finally {
            echo "method finally\n";
        }
    }
}

$cleanup = function ($name) {
    try {
        echo "closure try: ", $name, "\n";
        return "closure result " . $name;
    } finally {
        echo "closure finally: ", $name, "\n";
    }
};
$r = $cleanup("first");
echo $r, "\n";

$runner = new Runner();
$r = $runner->run(false);
echo "run ok: ", $r, "\n";
try {
    $r = $runner->run(true);
    echo "not reached: ", $r, "\n";
} catch (RuntimeException $e) {
    echo "caught from method: ", $e->getMessage(), "\n";
}

$thrower = function () {
    throw new LogicException("from closure");
};
try {
    try {
        echo "body\n";
    } finally {
        echo "calling closure from finally\n";
        try {
            $thrower();
        } catch (LogicException $e) {
            echo "caught inside finally: ", $e->getMessage(), "\n";
        }
    }
} catch (Exception $e) {
    echo "never\n";
}
echo "end\n";
