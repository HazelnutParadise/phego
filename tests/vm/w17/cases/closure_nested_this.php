<?php
// closure_nested_this: $this and class scope survive several nesting levels
class Level {
    private $tag = "L";

    private function mark($s) {
        return $s . "#";
    }
    public function build() {
        return function ($a) {
            return function ($b) use ($a) {
                return fn($c) => $this->tag . $a . $b . $c . $this->mark("m");
            };
        };
    }
    public function nestStatic() {
        return function () {
            return static function () {
                return isset($this) ? "has" : "none";
            };
        };
    }
    public function nestHasThis() {
        return function () {
            return isset($this) ? "outer has this" : "outer none";
        };
    }
}

$l = new Level();
$lvl = $l->build();
echo "three_deep: ", $lvl("a")("b")("c"), "\n";
echo "three_deep_again: ", $lvl("x")("y")("z"), "\n";
echo "outer_has_this: ", $l->nestHasThis()(), "\n";
$nest = $l->nestStatic()();
echo "static_inside_nonstatic: ", $nest(), "\n";
echo "static_isset_direct: ", isset($nest) ? "closure set" : "unset", "\n";

$shadowF = fn($v) => fn($w) => fn() => $v . $w;
echo "arrow_three_levels: ", $shadowF("p")("q")(), "\n";
