<?php
// try_nested: nested try blocks: inner does not match and outer does; inner matches; three levels; try inside catch
class OuterErr extends Exception {}
class InnerErr extends Exception {}

echo "case 1: inner misses, outer catches\n";
try {
    try {
        throw new OuterErr("o1");
    } catch (InnerErr $e) {
        echo "wrong: inner caught\n";
    }
    echo "not reached 1\n";
} catch (OuterErr $e) {
    echo "outer got: ", $e->getMessage(), "\n";
}

echo "case 2: inner matches\n";
try {
    try {
        throw new InnerErr("i2");
    } catch (InnerErr $e) {
        echo "inner got: ", $e->getMessage(), "\n";
    }
    echo "after inner catch\n";
} catch (Exception $e) {
    echo "wrong: outer caught\n";
}

echo "case 3: three levels\n";
try {
    try {
        try {
            throw new InnerErr("deep");
        } catch (OuterErr $e) {
            echo "wrong: level 3\n";
        }
    } catch (OuterErr $e) {
        echo "wrong: level 2\n";
    }
} catch (InnerErr $e) {
    echo "level 1 got: ", $e->getMessage(), "\n";
}

echo "case 4: try inside catch\n";
try {
    throw new OuterErr("first");
} catch (OuterErr $e) {
    echo "catch A: ", $e->getMessage(), "\n";
    try {
        throw new InnerErr("second");
    } catch (InnerErr $e2) {
        echo "nested in catch: ", $e2->getMessage(), "\n";
    }
    echo "still in catch A, e is: ", $e->getMessage(), "\n";
}

echo "case 5: exception thrown from catch body escapes\n";
try {
    try {
        throw new InnerErr("x");
    } catch (InnerErr $e) {
        throw new OuterErr("from catch");
    }
} catch (OuterErr $e) {
    echo "outer got rethrown: ", $e->getMessage(), "\n";
}
echo "end\n";
