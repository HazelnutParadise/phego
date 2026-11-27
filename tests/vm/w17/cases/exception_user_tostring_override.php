<?php
// exception_user_tostring_override: user __toString used by echo, concatenation and parent::__toString
class Custom extends Exception {
    public function __toString(): string {
        return "custom(" . $this->getMessage() . ")";
    }

    public function parentText(): string {
        return "parent=" . parent::__toString();
    }
}

class Quiet extends Exception {
    public function __toString(): string {
        return "";
    }
}

class Plain extends Exception {}

$c = new Custom("boom", 1);
echo "echo: ", $c, "\n";
echo "concat: " . $c . "\n";
echo "cast_equal: ", ((string)$c === "custom(boom)" ? 1 : 0), "\n";
echo "interpolated: {$c}\n";
echo $c->parentText(), "\n";

$q = new Quiet("x");
echo "quiet_empty: [", $q, "]\n";

$p = new Plain("default text");
echo "plain_first_line_class: ", $p::class, "\n";
echo "plain_has_default_text: ", ((string)$p === "" ? 0 : 1), "\n";
echo "custom_is_exception: ", ($c instanceof Exception ? 1 : 0), "\n";
echo "custom_code: ", $c->getCode(), "\n";
