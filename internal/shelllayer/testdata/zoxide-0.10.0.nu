export-env {
  $env.config = (
    $env.config?
    | default {}
    | upsert hooks { default {} }
    | upsert hooks.pre_prompt { default [] }
  )
  let __zoxide_hooked = (
    $env.config.hooks.pre_prompt | any { try { get __zoxide_hook } catch { false } }
  )
  if not $__zoxide_hooked {
    $env.config.hooks.pre_prompt = ($env.config.hooks.pre_prompt | append {
      __zoxide_hook: true,
      code: {|| ^zoxide add -- $env.PWD}
    })
  }
}
export def --env --wrapped __zoxide_z [...rest: directory] {
  let path = match $rest {
    [] => {'~'},
    [ '-' ] => {'-'},
    [ $arg ] if ($arg | path expand | path type) == 'dir' => {$arg}
    _ => {
      ^zoxide query --exclude $env.PWD -- ...$rest | str trim -r -c "\n"
    }
  }
  cd $path

}

# Jump to a directory using interactive search.
export def --env --wrapped __zoxide_zi [...rest: string] {
  cd $'(^zoxide query --interactive -- ...$rest | str trim -r -c "\n")'

}


export alias cd = __zoxide_z
export alias cdi = __zoxide_zi
