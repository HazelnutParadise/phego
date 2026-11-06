<?php
// REQ-018/019; T-035/036/037/038. zend_object_std_dtor destroys the property
// hash first (dynamic properties in insertion order; declared INDIRECT slots
// are skipped), then declared slots in declaration order.
$tests ??= [];
class W13R14Probe { public function __construct(public string $name) {} public function __destruct() { $GLOBALS['free_order'][] = $this->name; } }
#[AllowDynamicProperties] class W13R14Holder { public $declared1; public $declared2; }
#[AllowDynamicProperties] class W13R14Child extends W13R14Holder { public $declared3; }
$tests['r14-free-declared'] = function () {
    $GLOBALS['free_order'] = [];
    $o = new W13R14Holder; $o->declared1 = new W13R14Probe('declared1'); $o->declared2 = new W13R14Probe('declared2');
    unset($o);
    return $GLOBALS['free_order'];
};
$tests['r14-free-dynamic'] = function () {
    $GLOBALS['free_order'] = [];
    $o = new W13R14Holder; $o->declared2 = new W13R14Probe('declared2'); $o->declared1 = new W13R14Probe('declared1');
    $o->dynamic1 = new W13R14Probe('dynamic1'); $o->dynamic2 = new W13R14Probe('dynamic2');
    unset($o);
    return $GLOBALS['free_order'];
};
$tests['r14-free-subclass'] = function () {
    $GLOBALS['free_order'] = [];
    $o = new W13R14Child; $o->dynamic1 = new W13R14Probe('dynamic1'); $o->declared3 = new W13R14Probe('declared3');
    $o->declared1 = new W13R14Probe('declared1'); $o->dynamic2 = new W13R14Probe('dynamic2'); $o->declared2 = new W13R14Probe('declared2');
    unset($o);
    return $GLOBALS['free_order'];
};
$tests['r14-free-materialized'] = function () {
    $GLOBALS['free_order'] = [];
    $o = new W13R14Holder; $o->dynamic1 = new W13R14Probe('dynamic1'); $o->declared1 = new W13R14Probe('declared1');
    $o->dynamic2 = new W13R14Probe('dynamic2'); $o->declared2 = new W13R14Probe('declared2');
    foreach ($o as $name => $value) {}
    unset($name, $value);
    $keys = array_keys(get_object_vars($o));
    unset($o);
    return [$keys, $GLOBALS['free_order']];
};
$tests['r14-free-unset-redeclare'] = function () {
    $GLOBALS['free_order'] = [];
    $o = new W13R14Holder; $o->declared1 = new W13R14Probe('declared1'); $o->dynamic1 = new W13R14Probe('dynamic1');
    unset($o->declared1); $o->declared1 = new W13R14Probe('declared1b'); $o->dynamic2 = new W13R14Probe('dynamic2');
    unset($o);
    return $GLOBALS['free_order'];
};
