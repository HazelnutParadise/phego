<?php
// throw_from_magic_methods: each magic method throws and the exception is caught by the caller
class Magic {
    public function __get($name) {
        throw new Exception("get " . $name);
    }
    public function __set($name, $value) {
        throw new Exception("set " . $name);
    }
    public function __isset($name) {
        throw new Exception("isset " . $name);
    }
    public function __unset($name) {
        throw new Exception("unset " . $name);
    }
    public function __call($name, $args) {
        throw new Exception("call " . $name);
    }
    public static function __callStatic($name, $args) {
        throw new Exception("static " . $name);
    }
    public function __toString(): string {
        throw new Exception("tostring");
    }
    public function __invoke($x) {
        throw new Exception("invoke " . $x);
    }
}

$m = new Magic();

try {
    $v = $m->foo;
    echo "not reached get\n";
} catch (Exception $e) {
    echo "__get: ", $e->getMessage(), "\n";
}

try {
    $m->bar = 1;
    echo "not reached set\n";
} catch (Exception $e) {
    echo "__set: ", $e->getMessage(), "\n";
}

try {
    $flag = isset($m->baz);
    echo "not reached isset\n";
} catch (Exception $e) {
    echo "__isset: ", $e->getMessage(), "\n";
}

try {
    unset($m->qux);
    echo "not reached unset\n";
} catch (Exception $e) {
    echo "__unset: ", $e->getMessage(), "\n";
}

try {
    $m->doIt(1);
    echo "not reached call\n";
} catch (Exception $e) {
    echo "__call: ", $e->getMessage(), "\n";
}

try {
    Magic::stat();
    echo "not reached callStatic\n";
} catch (Exception $e) {
    echo "__callStatic: ", $e->getMessage(), "\n";
}

try {
    $s = (string)$m;
    echo "not reached tostring\n";
} catch (Exception $e) {
    echo "__toString: ", $e->getMessage(), "\n";
}

try {
    $r = $m(5);
    echo "not reached invoke\n";
} catch (Exception $e) {
    echo "__invoke: ", $e->getMessage(), "\n";
}
echo "end\n";
