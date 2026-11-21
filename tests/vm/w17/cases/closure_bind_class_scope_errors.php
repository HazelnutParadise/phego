<?php
$c = function () {
    return "ok";
};
$o = new stdClass;

$r1 = $c->bindTo($o, 'NoSuchClass');
echo "missing_class_is_null: ", $r1 === null ? "yes" : "no", "\n";

$r2 = $c->bindTo($o, 'stdClass');
echo "internal_scope_is_null: ", $r2 === null ? "yes" : "no", "\n";

$r3 = $c->bindTo($o);
echo "no_scope_bind_is_null: ", $r3 === null ? "yes" : "no", "\n";
echo "no_scope_bind_call: ", $r3(), "\n";

$tc = function () {
    return "this class: " . $this::class;
};
$r4 = $tc->bindTo(new stdClass);
echo "object_class_inside: ", $r4(), "\n";

$selfc = function () {
    return "self is " . self::class;
};
$r6 = $selfc->bindTo(new stdClass);
echo "bound_self_class: ", $r6(), "\n";

$r5 = $tc->bindTo(new stdClass, 'static');
echo "static_scope_bind_is_null: ", $r5 === null ? "yes" : "no", "\n";
echo "end\n";
