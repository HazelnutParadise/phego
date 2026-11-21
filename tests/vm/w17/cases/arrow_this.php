<?php
class Base {
    const NAME = "base";
    private $secret = "s1";
    protected $level = 2;

    public function arrows() {
        $get = fn () => $this->secret;
        $bump = fn ($n) => $this->level + $n;
        return [$get, $bump];
    }

    public function staticArrow() {
        return static fn () => isset($this) ? "has this" : "no this";
    }

    public function names() {
        return fn () => self::NAME . "/" . static::NAME . "/" . static::class;
    }

    public static function sname() {
        return static fn () => self::NAME . "/" . static::class;
    }

    public function setSecret($v) {
        $set = fn ($x) => $this->secret = $x;
        $set($v);
        return $this->secret;
    }
}

class Child extends Base {
    const NAME = "child";

    public function inherited() {
        return fn () => parent::NAME;
    }
}

$b = new Base;
$arrs = $b->arrows();
echo "get_secret: ", $arrs[0](), "\n";
echo "bump_with_level: ", $arrs[1](40), "\n";
echo "static_arrow_this: ", $b->staticArrow()(), "\n";
echo "names_base: ", $b->names()(), "\n";
$c = new Child;
echo "names_child: ", $c->names()(), "\n";
echo "sname_called_on_child: ", Child::sname()(), "\n";
echo "set_secret_returns: ", $b->setSecret("s2"), "\n";
echo "get_after_set: ", $arrs[0](), "\n";
echo "inherited_parent_name: ", $c->inherited()(), "\n";
