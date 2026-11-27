<?php
// exception_user_subclass_ctor: user subclasses with an overridden constructor, a reordered parent call and a typed extra property
class Silent extends Exception {
    public function __construct() {
        // parent constructor is intentionally not called
    }
}

class Reordered extends Exception {
    public function __construct($code, $message) {
        parent::__construct($message, $code);
    }
}

class Typed extends Exception {
    public int $retries = 0;

    public function __construct($message, $retries) {
        parent::__construct($message);
        $this->retries = $retries;
    }

    public function getRetries(): int {
        return $this->retries;
    }
}

$s = new Silent();
echo "silent_message: [", $s->getMessage(), "]\n";
echo "silent_code: ", $s->getCode(), "\n";
echo "silent_line: ", $s->getLine(), "\n";
echo "silent_file: ", $s->getFile(), "\n";
echo "silent_previous_null: ", ($s->getPrevious() === null ? 1 : 0), "\n";

$r = new Reordered(77, "reordered");
echo "reordered_message: ", $r->getMessage(), "\n";
echo "reordered_code: ", $r->getCode(), "\n";

$t = new Typed("typed message", 3);
echo "typed_message: ", $t->getMessage(), "\n";
echo "typed_retries: ", $t->getRetries(), "\n";
echo "typed_retries_prop: ", $t->retries, "\n";
echo "typed_line: ", $t->getLine(), "\n";
echo "typed_class: ", $t::class, "\n";
echo "typed_is_exception: ", ($t instanceof Exception ? 1 : 0), "\n";
