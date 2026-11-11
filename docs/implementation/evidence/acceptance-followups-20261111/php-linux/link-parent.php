<?php
// PHP 8.5.12 on links whose target leaves a directory with .. and comes
// back (W-20). Mirrors TestAbsoluteLinkChainParentUsesNewMount and
// TestReviewFixAbsoluteSymlinkMounts in internal/policy.
$base = sys_get_temp_dir() . '/b';
foreach (['app/sub', 'other', 'outside', 'app2'] as $d) mkdir("$base/$d", 0700, true);
foreach (['app/file', 'app/sub/file', 'other/file', 'outside/file', 'app2/file'] as $f) file_put_contents("$base/$f", $f);
symlink("$base/app/sub", "$base/other/toapp");
$links = [
    'return' => "$base/other/toapp/../../app/file",
    'sibling' => "$base/other/toapp/../../other/file",
    'wander' => "$base/other/toapp/../../outside/../app/file",
    'escape' => "$base/other/toapp/../../outside/file",
    'rel' => "../other/toapp/../file",
    'escape-return' => "$base/app/../app/file",
    'target-boundary' => "$base/other/../app/file",
    'parent-outside' => "$base/app/../outside/file",
    'parent-prefix' => "$base/app/../app2/file",
];
echo PHP_VERSION, ' ', PHP_OS, ' ', php_uname('m'), "\n";
foreach ($links as $name => $target) {
    symlink($target, "$base/app/$name");
    echo "/app/$name -> ", str_replace($base, '<base>', $target), ": ", var_export(file_get_contents("$base/app/$name"), true), "\n";
}
