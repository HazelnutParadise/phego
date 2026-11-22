<?php
// exception_user_interface: a user interface extending Throwable, implemented by an Exception subclass and caught by it
interface MyThrowable extends Throwable {
    public function tag(): string;
}

class Fancy extends Exception implements MyThrowable {
    public function tag(): string {
        return "fancy:" . $this->getMessage();
    }
}

class Plain extends Exception {}

class Other extends Error {}

function raise($which) {
    if ($which == 1) {
        throw new Fancy("one");
    }
    if ($which == 2) {
        throw new Plain("two");
    }
    throw new Other("three");
}

for ($i = 1; $i <= 3; $i++) {
    try {
        raise($i);
    } catch (MyThrowable $t) {
        echo "caught_mythrowable_", $i, ": ", $t->tag(), "\n";
    } catch (Throwable $t) {
        echo "caught_throwable_", $i, ": ", $t::class, "\n";
    }
}

$f = new Fancy("x");
echo "fancy_is_mythrowable: ", ($f instanceof MyThrowable ? 1 : 0), "\n";
echo "fancy_is_throwable: ", ($f instanceof Throwable ? 1 : 0), "\n";
echo "fancy_is_exception: ", ($f instanceof Exception ? 1 : 0), "\n";
echo "fancy_is_error: ", ($f instanceof Error ? 1 : 0), "\n";
$pl = new Plain("y");
echo "plain_is_mythrowable: ", ($pl instanceof MyThrowable ? 1 : 0), "\n";
$o = new Other("z");
echo "other_is_mythrowable: ", ($o instanceof MyThrowable ? 1 : 0), "\n";
echo "fancy_tag_direct: ", $f->tag(), "\n";
