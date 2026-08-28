const PanelLayout: React.FC<{ title: string; children: React.ReactNode }> = ({
  title,
  children
}) => {
  return (
    <>
      <div className="pt-4 px-5 text-gray-900 text-sm font-bold">{title}</div>
      <div className="p-5">{children}</div>
    </>
  )
}

export default PanelLayout
